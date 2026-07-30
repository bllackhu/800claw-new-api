package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

// TokenRollingRequestRedisKey is the token-scoped Redis ZSET for rolling request-count enforcement.
func TokenRollingRequestRedisKey(tokenId int) string {
	return fmt.Sprintf("token:rq:events:%d", tokenId)
}

// LegacyPoolTokenRollingRequestRedisKey is the pre-migration pool-scoped key for a token.
func LegacyPoolTokenRollingRequestRedisKey(poolId, tokenId int) string {
	return fmt.Sprintf("pool:rq:events:%d:token:%d", poolId, tokenId)
}

// PoolRollingRequestRedisKey resolves the Redis key for rolling request quota by scope.
// Token scope uses a token-scoped key; user scope remains pool-scoped.
func PoolRollingRequestRedisKey(poolId int, scopeKey string) string {
	if strings.HasPrefix(scopeKey, "token:") {
		if tokenId, err := strconv.Atoi(strings.TrimPrefix(scopeKey, "token:")); err == nil && tokenId > 0 {
			return TokenRollingRequestRedisKey(tokenId)
		}
	}
	return fmt.Sprintf("pool:rq:events:%d:%s", poolId, scopeKey)
}

func maxTokenScopeRequestWindowSeconds(poolId int) int {
	policies, err := GetPoolQuotaPolicies(poolId, PoolQuotaMetricRequestCount, PoolQuotaScopeToken)
	if err != nil {
		return 0
	}
	maxWindow := 0
	for _, p := range policies {
		if p == nil || !p.Enabled || p.WindowSeconds <= 0 || p.LimitCount <= 0 {
			continue
		}
		if p.WindowSeconds > maxWindow {
			maxWindow = p.WindowSeconds
		}
	}
	return maxWindow
}

// MergeTokenRollingRequestEvents unions legacy pool-scoped events into the token-scoped key.
func MergeTokenRollingRequestEvents(oldPoolId, tokenId, maxWindowSeconds int) error {
	if !common.RedisEnabled || common.RDB == nil || tokenId <= 0 || oldPoolId <= 0 {
		return nil
	}
	dest := TokenRollingRequestRedisKey(tokenId)
	legacy := LegacyPoolTokenRollingRequestRedisKey(oldPoolId, tokenId)
	ctx := context.Background()
	if exists, err := common.RDB.Exists(ctx, legacy).Result(); err != nil {
		return err
	} else if exists == 0 {
		return nil
	}
	if err := common.RDB.ZUnionStore(ctx, dest, &redis.ZStore{
		Keys:      []string{legacy, dest},
		Aggregate: "MAX",
	}).Err(); err != nil {
		return err
	}
	if maxWindowSeconds <= 0 {
		maxWindowSeconds = maxTokenScopeRequestWindowSeconds(oldPoolId)
	}
	if maxWindowSeconds <= 0 {
		maxWindowSeconds = 30 * 24 * 3600
	}
	return common.RDB.Expire(ctx, dest, time.Duration(maxWindowSeconds+60)*time.Second).Err()
}

// FixedWindowCounterKey builds the Redis key for a fixed-window counter bucket.
// Buckets are epoch-aligned: floor(nowUnix / windowSeconds) * windowSeconds.
func FixedWindowCounterKey(scopeKey string, windowSeconds int, nowUnix int64) string {
	bucket := nowUnix / int64(windowSeconds)
	return fmt.Sprintf("token:rq:fixed:%s:%d:%d", scopeKey, windowSeconds, bucket)
}

// FixedWindowBucketNumber returns the epoch-aligned bucket number for a given timestamp.
func FixedWindowBucketNumber(nowUnix int64, windowSeconds int) int64 {
	return nowUnix / int64(windowSeconds)
}

// FixedWindowResetAt returns the Unix timestamp when the current bucket resets (end of the epoch-aligned window).
func FixedWindowResetAt(nowUnix int64, windowSeconds int) int64 {
	bucket := nowUnix / int64(windowSeconds)
	return (bucket + 1) * int64(windowSeconds)
}

// FixedWindowResetInSeconds returns the number of seconds until the next reset.
func FixedWindowResetInSeconds(nowUnix int64, windowSeconds int) int64 {
	resetAt := FixedWindowResetAt(nowUnix, windowSeconds)
	return resetAt - nowUnix
}

// GetFixedWindowCount returns the current bucket counter for the given scope/window.
// Returns 0 when Redis is disabled or the key is absent (fresh bucket).
func GetFixedWindowCount(ctx context.Context, scopeKey string, windowSeconds int, nowUnix int64) (int64, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return 0, nil
	}
	key := FixedWindowCounterKey(scopeKey, windowSeconds, nowUnix)
	val, err := common.RDB.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// ResetFixedWindowCount deletes the current bucket counter for the given scope/window.
// Absent keys are treated as success (already zero). No-op when Redis is disabled.
func ResetFixedWindowCount(ctx context.Context, scopeKey string, windowSeconds int, nowUnix int64) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}
	key := FixedWindowCounterKey(scopeKey, windowSeconds, nowUnix)
	return common.RDB.Del(ctx, key).Err()
}
