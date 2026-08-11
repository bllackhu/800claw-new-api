package middleware

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/go-redis/redis/v8"
	"github.com/gin-gonic/gin"
)

var fixedWindowScriptSHA string

func InitFixedWindowScript() {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	sha, err := common.RDB.ScriptLoad(context.Background(), limiter.FixedWindowEnforceScript).Result()
	if err != nil {
		common.SysLog(fmt.Sprintf("Failed to load fixed window enforce script: %v", err))
		return
	}
	fixedWindowScriptSHA = sha
}

type poolQuotaPolicyLoader func(poolId int, metric string, scopeType string) ([]*model.PoolQuotaPolicy, error)

func filterValidPoolQuotaPolicies(policies []*model.PoolQuotaPolicy) ([]*model.PoolQuotaPolicy, int) {
	validPolicies := make([]*model.PoolQuotaPolicy, 0, len(policies))
	maxWindowSeconds := 0
	for _, p := range policies {
		if p == nil || p.WindowSeconds <= 0 || p.LimitCount <= 0 {
			continue
		}
		validPolicies = append(validPolicies, p)
		if p.WindowSeconds > maxWindowSeconds {
			maxWindowSeconds = p.WindowSeconds
		}
	}
	return validPolicies, maxWindowSeconds
}

// loadPoolQuotaScopePoliciesAndScopeKey resolves active policies and redis scope key.
//
// Scope precedence:
// 1) token scope, if token-scope policies exist
// 2) user scope
//
// If token-scope policies exist but token id is missing/invalid, it falls back
// to user scope key as a safe default.
func loadPoolQuotaScopePoliciesAndScopeKey(c *gin.Context, poolId int, loader poolQuotaPolicyLoader) ([]*model.PoolQuotaPolicy, string, int, error) {
	tokenPolicies, err := loader(poolId, model.PoolQuotaMetricRequestCount, model.PoolQuotaScopeToken)
	if err != nil {
		return nil, "", 0, err
	}
	if validTokenPolicies, tokenMaxWindowSeconds := filterValidPoolQuotaPolicies(tokenPolicies); len(validTokenPolicies) > 0 {
		tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
		scopeKey := ""
		if tokenId > 0 {
			scopeKey = "token:" + strconv.Itoa(tokenId)
		} else {
			// Fallback: token-scope policy exists but token identity is unavailable.
			userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
			scopeKey = "user:" + strconv.Itoa(userId)
		}
		common.SetContextKey(c, constant.ContextKeyPoolScopeKey, scopeKey)
		return validTokenPolicies, scopeKey, tokenMaxWindowSeconds, nil
	}

	userPolicies, err := loader(poolId, model.PoolQuotaMetricRequestCount, model.PoolQuotaScopeUser)
	if err != nil {
		return nil, "", 0, err
	}
	validUserPolicies, userMaxWindowSeconds := filterValidPoolQuotaPolicies(userPolicies)
	if len(validUserPolicies) == 0 || userMaxWindowSeconds <= 0 {
		return nil, "", 0, nil
	}

	scopeKey := common.GetContextKeyString(c, constant.ContextKeyPoolScopeKey)
	if scopeKey == "" {
		userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
		scopeKey = "user:" + strconv.Itoa(userId)
		common.SetContextKey(c, constant.ContextKeyPoolScopeKey, scopeKey)
	}
	return validUserPolicies, scopeKey, userMaxWindowSeconds, nil
}

func PoolRollingQuota() func(c *gin.Context) {
	return func(c *gin.Context) {
		if !common.PoolEnabled || !common.PoolQuotaEnabled || !common.PoolRollingWindowEnabled {
			c.Next()
			return
		}

		if !common.RedisEnabled || common.RDB == nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "pool rolling quota requires redis")
			return
		}

		poolId := common.GetContextKeyInt(c, constant.ContextKeyPoolId)
		if poolId <= 0 {
			c.Next()
			return
		}

		validPolicies, scopeKey, maxWindowSeconds, err := loadPoolQuotaScopePoliciesAndScopeKey(c, poolId, model.GetPoolQuotaPolicies)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "failed to load pool quota policies")
			return
		}

		if len(validPolicies) == 0 || maxWindowSeconds <= 0 {
			c.Next()
			return
		}

		// Determine which mode to use for this request
		pool, _ := model.GetPoolById(poolId)
		useFixed := common.PoolFixedWindowEnabled &&
			pool != nil &&
			pool.RateLimitMode == model.PoolRateLimitModeFixed

		if useFixed {
			enforceFixedWindow(c, validPolicies, scopeKey)
		} else {
			enforceSlidingWindow(c, poolId, validPolicies, scopeKey, maxWindowSeconds)
		}
	}
}

func enforceSlidingWindow(c *gin.Context, poolId int, validPolicies []*model.PoolQuotaPolicy, scopeKey string, maxWindowSeconds int) {
	requestId := c.GetString(common.RequestIdKey)
	if requestId == "" {
		requestId = fmt.Sprintf("%d-%s", time.Now().UnixNano(), common.GetRandomString(8))
	}

	redisKey := model.PoolRollingRequestRedisKey(poolId, scopeKey)
	nowMs := time.Now().UnixMilli()
	ctx := context.Background()

	if err := trimPoolWindowEvents(ctx, redisKey, maxWindowSeconds, nowMs); err != nil {
		abortWithOpenAiMessage(c, http.StatusInternalServerError, "failed to trim pool quota events")
		return
	}

	for _, p := range validPolicies {
		usedCount, countErr := countRollingWindowEvents(ctx, redisKey, p.WindowSeconds, nowMs)
		if countErr != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "failed to count pool quota events")
			return
		}
		if usedCount >= int64(p.LimitCount) {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("pool request limit exceeded: %d requests in %d seconds", p.LimitCount, p.WindowSeconds))
			return
		}
	}

	if err := reservePoolRequestEvent(ctx, redisKey, requestId, nowMs, maxWindowSeconds); err != nil {
		abortWithOpenAiMessage(c, http.StatusInternalServerError, "failed to reserve pool quota event")
		return
	}

	c.Next()
	if c.Writer != nil && c.Writer.Status() >= http.StatusBadRequest {
		_ = common.RDB.ZRem(ctx, redisKey, requestId).Err()
	}
}

func enforceFixedWindow(c *gin.Context, policies []*model.PoolQuotaPolicy, scopeKey string) {
	ctx := context.Background()
	nowUnix := time.Now().Unix()

	// Resolve the per-request cost rate from the original model.
	modelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	costRate := ratio_setting.GetModelCostRate(modelName)
	if costRate <= 0 || math.IsNaN(costRate) || math.IsInf(costRate, 0) {
		costRate = 1.0
	}

	// Build keys, limits, and per-key increment deltas for the Lua script
	keys := make([]string, 0, len(policies))
	limits := make([]string, 0, len(policies))
	ttls := make([]string, 0, len(policies))
	deltas := make([]string, 0, len(policies))

	for _, p := range policies {
		key := model.FixedWindowCounterKey(scopeKey, p.WindowSeconds, nowUnix)
		keys = append(keys, key)
		limits = append(limits, strconv.Itoa(p.LimitCount))
		ttls = append(ttls, strconv.Itoa(p.WindowSeconds))
		deltas = append(deltas, strconv.FormatFloat(costRate, 'f', -1, 64))
	}

	// Atomic enforcement via Lua script
	argv := append(append(limits, ttls...), deltas...)
	interfaceArgs := make([]interface{}, len(argv))
	for i, v := range argv {
		interfaceArgs[i] = v
	}
	result, err := common.RDB.EvalSha(ctx, fixedWindowScriptSHA, keys, interfaceArgs...).Int()
	if err != nil {
		common.SysLog(fmt.Sprintf("fixed window enforce script error: %v", err))
		abortWithOpenAiMessage(c, http.StatusInternalServerError, "pool fixed window enforcement failed")
		return
	}
	if result == 0 {
		abortWithOpenAiMessage(c, http.StatusTooManyRequests, "pool request limit exceeded")
		return
	}

	// Store keys and deltas for potential rollback on failed request
	c.Set("pool_fixed_window_keys", keys)
	c.Set("pool_fixed_window_deltas", deltas)
	c.Next()

	if c.Writer != nil && c.Writer.Status() >= http.StatusBadRequest {
		deltaList, _ := c.Get("pool_fixed_window_deltas")
		for i, k := range keys {
			delta := 1.0
			if dl, ok := deltaList.([]string); ok && i < len(dl) {
				if parsed, err := strconv.ParseFloat(dl[i], 64); err == nil {
					delta = parsed
				}
			}
			_ = common.RDB.IncrByFloat(ctx, k, -delta).Err()
		}
	}
}

func trimPoolWindowEvents(ctx context.Context, redisKey string, maxWindowSeconds int, nowMs int64) error {
	trimBeforeMs := nowMs - int64(maxWindowSeconds)*1000
	return common.RDB.ZRemRangeByScore(ctx, redisKey, "-inf", fmt.Sprintf("%d", trimBeforeMs)).Err()
}

func countRollingWindowEvents(ctx context.Context, redisKey string, windowSeconds int, nowMs int64) (int64, error) {
	windowStartMs := nowMs - int64(windowSeconds)*1000
	return common.RDB.ZCount(ctx, redisKey, fmt.Sprintf("(%d", windowStartMs), "+inf").Result()
}

func reservePoolRequestEvent(ctx context.Context, redisKey string, requestId string, nowMs int64, maxWindowSeconds int) error {
	if err := common.RDB.ZAdd(ctx, redisKey, &redis.Z{
		Score:  float64(nowMs),
		Member: requestId,
	}).Err(); err != nil {
		return err
	}
	return common.RDB.Expire(ctx, redisKey, time.Duration(maxWindowSeconds+60)*time.Second).Err()
}