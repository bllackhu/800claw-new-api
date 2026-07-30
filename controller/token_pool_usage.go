package controller

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const tokenPoolUsageDataSource = "token_llm_rollups"

const (
	poolDisplayNameFallback = "Pool"
)

const (
	tokenPoolUsageReasonNoResolvedPool     = "no_resolved_pool"
	tokenPoolUsageReasonWindowNotRetained  = "window_not_retained"
	tokenPoolUsageReasonTokenScopeDisabled = "token_scope_not_enabled"
	tokenPoolUsageReasonUserScopeOnly      = "user_scope_only"
)

var defaultTokenPoolUsageWindows = []string{"5h", "7d", "30d"}
var buildTokenPoolUsageItemFunc = buildTokenPoolUsageItem

type TokenPoolUsageBatchRequest struct {
	Ids     []int    `json:"ids"`
	Windows []string `json:"windows"`
}

type TokenPoolUsageResetRequest struct {
	Window string `json:"window"`
}

type tokenPoolUsageResetDeps struct {
	resolvePool          func(token *model.Token) (*model.Pool, error)
	loadPolicies         func(poolId int, scopeType string) ([]*model.PoolQuotaPolicy, error)
	resetFixedWindow     func(ctx context.Context, scopeKey string, windowSeconds int, nowUnix int64) error
	recordManageLog      func(userId int, content string)
}

type TokenPoolUsageWindow struct {
	Window        string `json:"window"`
	WindowSeconds int    `json:"window_seconds"`
	Available     bool   `json:"available"`
	Count         *int64 `json:"count,omitempty"`
	LimitCount    *int64 `json:"limit_count,omitempty"`
	Reason        string `json:"reason,omitempty"`
	// Fixed-window fields (only present when pool.rate_limit_mode = "fixed")
	ResetAt        *int64 `json:"reset_at,omitempty"`
	ResetInSeconds *int64 `json:"reset_in_seconds,omitempty"`
}

type TokenPoolUsageItem struct {
	TokenId                int                              `json:"token_id"`
	PoolId                 int                              `json:"pool_id,omitempty"`
	PoolName               string                           `json:"pool_name,omitempty"`
	ScopeType              string                           `json:"scope_type,omitempty"`
	DataSource             string                           `json:"data_source"`
	RetentionWindowSeconds int                              `json:"retention_window_seconds,omitempty"`
	TokenScopeEnabled      bool                             `json:"token_scope_enabled"`
	RateLimitMode          string                           `json:"rate_limit_mode,omitempty"`
	Usage                  map[string]*TokenPoolUsageWindow `json:"usage"`
	LlmTokenUsage          *TokenPoolLLMTokenUsage          `json:"llm_token_usage,omitempty"`
}

const tokenPoolLLMTokenDataSource = "token_llm_rollups"

// TokenPoolLLMTokenWindow is rolling-window LLM token totals from hourly rollup buckets for this API token.
type TokenPoolLLMTokenWindow struct {
	Window           string `json:"window"`
	WindowSeconds    int    `json:"window_seconds"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

// TokenPoolLLMTokenLifetime is all-time LLM token totals stored on the tokens row.
type TokenPoolLLMTokenLifetime struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// TokenPoolLLMTokenUsage bundles per-window and lifetime aggregates for GET /api/usage/token/pool.
type TokenPoolLLMTokenUsage struct {
	DataSource string                              `json:"data_source"`
	ByWindow   map[string]*TokenPoolLLMTokenWindow `json:"by_window"`
	Lifetime   TokenPoolLLMTokenLifetime           `json:"lifetime"`
}

// TokenPoolSubscriptionInfo is subscription and billing metadata for mobile/display clients.
type TokenPoolSubscriptionInfo struct {
	Active                  bool    `json:"active"`
	PeriodStart             int64   `json:"period_start"`
	PeriodEnd               int64   `json:"period_end"`
	MonthlyPriceCny         float64 `json:"monthly_price_cny"`
	BillingCurrency         string  `json:"billing_currency"`
	BillingPeriodSeconds    int64   `json:"billing_period_seconds"`
	CheckoutAvailable       bool    `json:"checkout_available"`
	RequirePoolSubscription bool    `json:"require_pool_subscription"`
}

// TokenPoolResolvedPoolSummary is display-oriented pool metadata (no id required for UI labels).
type TokenPoolResolvedPoolSummary struct {
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	MonthlyPriceCny float64 `json:"monthly_price_cny"`
}

func poolDisplayName(pool *model.Pool) string {
	if pool == nil {
		return poolDisplayNameFallback
	}
	if name := strings.TrimSpace(pool.Name); name != "" {
		return name
	}
	return poolDisplayNameFallback
}

func applyTokenPoolUsagePoolName(item *TokenPoolUsageItem, pool *model.Pool) {
	if item == nil || pool == nil || pool.Id <= 0 {
		return
	}
	item.PoolName = poolDisplayName(pool)
}

func buildTokenPoolSubscriptionInfo(ctx context.Context, token *model.Token, pool *model.Pool) (*TokenPoolSubscriptionInfo, error) {
	if pool == nil || pool.Id <= 0 {
		return nil, nil
	}
	period := pool.BillingPeriodSeconds
	if period <= 0 {
		period = 30 * 24 * 3600
	}
	cur := pool.BillingCurrency
	if cur == "" {
		cur = "CNY"
	}
	info := &TokenPoolSubscriptionInfo{
		MonthlyPriceCny:         pool.MonthlyPriceCny,
		BillingCurrency:         cur,
		BillingPeriodSeconds:    period,
		CheckoutAvailable:       model.PoolRequiresPaidSubscription(pool),
		RequirePoolSubscription: false,
	}
	if token != nil {
		info.RequirePoolSubscription = token.RequirePoolSubscription
	}
	if token == nil || token.Id <= 0 {
		return info, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := common.GetTimestamp()
	sub, err := model.GetTokenPoolSubscription(token.Id, pool.Id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if _, reconcileErr := service.MaybeReconcilePendingPoolSubscription(ctx, token.Id, pool.Id); reconcileErr != nil {
				logger.LogError(ctx, "pool subscription status lazy reconcile failed: "+reconcileErr.Error())
			} else {
				sub, err = model.GetTokenPoolSubscription(token.Id, pool.Id)
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, err
				}
			}
			if sub == nil {
				return info, nil
			}
		} else {
			return nil, err
		}
	}
	info.PeriodStart = sub.PeriodStart
	info.PeriodEnd = sub.PeriodEnd
	if sub.PeriodEnd >= now {
		info.Active = true
		return info, nil
	}
	if _, reconcileErr := service.MaybeReconcilePendingPoolSubscription(ctx, token.Id, pool.Id); reconcileErr != nil {
		logger.LogError(ctx, "pool subscription status lazy reconcile failed: "+reconcileErr.Error())
	} else {
		sub, err = model.GetTokenPoolSubscription(token.Id, pool.Id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return info, nil
			}
			return nil, err
		}
		info.PeriodStart = sub.PeriodStart
		info.PeriodEnd = sub.PeriodEnd
		if sub.PeriodEnd >= now {
			info.Active = true
		}
	}
	return info, nil
}

func buildTokenPoolResolvedPoolSummary(pool *model.Pool) *TokenPoolResolvedPoolSummary {
	if pool == nil || pool.Id <= 0 {
		return nil
	}
	return &TokenPoolResolvedPoolSummary{
		Name:            poolDisplayName(pool),
		Description:     strings.TrimSpace(pool.Description),
		MonthlyPriceCny: pool.MonthlyPriceCny,
	}
}

func buildTokenPoolLLMTokenUsage(token *model.Token, windows []string, windowSeconds map[string]int) (*TokenPoolLLMTokenUsage, error) {
	if token == nil {
		return nil, errors.New("token is nil")
	}
	out := &TokenPoolLLMTokenUsage{
		DataSource: tokenPoolLLMTokenDataSource,
		ByWindow:   make(map[string]*TokenPoolLLMTokenWindow, len(windows)),
	}
	now := time.Now().Unix()
	for _, w := range windows {
		sec := windowSeconds[w]
		since := now - int64(sec)
		prompt, completion, err := model.SumTokenLLMUsageBucketsByTokenSince(token.Id, since)
		if err != nil {
			return nil, err
		}
		out.ByWindow[w] = &TokenPoolLLMTokenWindow{
			Window:           w,
			WindowSeconds:    sec,
			PromptTokens:     prompt,
			CompletionTokens: completion,
			TotalTokens:      prompt + completion,
		}
	}
	lp := token.LlmPromptTokensTotal
	lc := token.LlmCompletionTokensTotal
	out.Lifetime = TokenPoolLLMTokenLifetime{
		PromptTokens:     lp,
		CompletionTokens: lc,
		TotalTokens:      lp + lc,
	}
	return out, nil
}

type tokenPoolUsageBuilderDeps struct {
	resolvePool                func(token *model.Token) (*model.Pool, error)
	loadPolicies               func(poolId int, scopeType string) ([]*model.PoolQuotaPolicy, error)
	countRequestsByToken       func(tokenId int, windowSeconds int) (int64, error)
	countFixedRequestsByToken  func(tokenId int, windowSeconds int) (int64, error)
}

func normalizeTokenPoolUsageWindows(input []string) ([]string, map[string]int, error) {
	if len(input) == 0 {
		input = defaultTokenPoolUsageWindows
	}
	seen := make(map[string]struct{}, len(input))
	windows := make([]string, 0, len(input))
	windowSeconds := make(map[string]int, len(input))
	for _, item := range input {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seconds, err := parseRollingWindow(item)
		if err != nil {
			return nil, nil, err
		}
		seen[item] = struct{}{}
		windows = append(windows, item)
		windowSeconds[item] = seconds
	}
	if len(windows) == 0 {
		return nil, nil, errors.New("at least one valid window is required")
	}
	sort.SliceStable(windows, func(i, j int) bool {
		return windowSeconds[windows[i]] < windowSeconds[windows[j]]
	})
	return windows, windowSeconds, nil
}

func filterValidRequestCountPolicies(policies []*model.PoolQuotaPolicy) ([]*model.PoolQuotaPolicy, int) {
	validPolicies := make([]*model.PoolQuotaPolicy, 0, len(policies))
	maxWindowSeconds := 0
	for _, policy := range policies {
		if policy == nil || !policy.Enabled || policy.WindowSeconds <= 0 || policy.LimitCount <= 0 {
			continue
		}
		validPolicies = append(validPolicies, policy)
		if policy.WindowSeconds > maxWindowSeconds {
			maxWindowSeconds = policy.WindowSeconds
		}
	}
	return validPolicies, maxWindowSeconds
}

func buildUnavailableTokenPoolUsage(item *TokenPoolUsageItem, windows []string, windowSeconds map[string]int, reason string) *TokenPoolUsageItem {
	item.Usage = make(map[string]*TokenPoolUsageWindow, len(windows))
	for _, window := range windows {
		item.Usage[window] = &TokenPoolUsageWindow{
			Window:        window,
			WindowSeconds: windowSeconds[window],
			Available:     false,
			Reason:        reason,
		}
	}
	return item
}


func buildTokenPoolUsageItem(token *model.Token, windows []string, windowSeconds map[string]int) (*TokenPoolUsageItem, error) {
	deps := tokenPoolUsageBuilderDeps{
		resolvePool: func(token *model.Token) (*model.Pool, error) {
			return model.ResolvePoolForContext(token.UserId, token.Id, token.Group)
		},
		loadPolicies: func(poolId int, scopeType string) ([]*model.PoolQuotaPolicy, error) {
			return model.GetPoolQuotaPolicies(poolId, model.PoolQuotaMetricRequestCount, scopeType)
		},
		countRequestsByToken: func(tokenId int, windowSeconds int) (int64, error) {
			since := time.Now().Unix() - int64(windowSeconds)
			return model.SumTokenLLMUsageBucketRequestCountByTokenSince(tokenId, since)
		},
		countFixedRequestsByToken: func(tokenId int, windowSeconds int) (int64, error) {
			scopeKey := "token:" + strconv.Itoa(tokenId)
			return model.GetFixedWindowCount(context.Background(), scopeKey, windowSeconds, time.Now().Unix())
		},
	}
	return buildTokenPoolUsageItemWithDeps(token, windows, windowSeconds, deps)
}

func buildTokenPoolUsageItemWithDeps(token *model.Token, windows []string, windowSeconds map[string]int, deps tokenPoolUsageBuilderDeps) (*TokenPoolUsageItem, error) {
	item := &TokenPoolUsageItem{
		TokenId:    token.Id,
		DataSource: tokenPoolUsageDataSource,
		Usage:      make(map[string]*TokenPoolUsageWindow, len(windows)),
	}
	pool, err := deps.resolvePool(token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return buildUnavailableTokenPoolUsage(item, windows, windowSeconds, tokenPoolUsageReasonNoResolvedPool), nil
		}
		return nil, err
	}
	if pool == nil {
		return buildUnavailableTokenPoolUsage(item, windows, windowSeconds, tokenPoolUsageReasonNoResolvedPool), nil
	}
	item.PoolId = pool.Id
	item.PoolName = poolDisplayName(pool)
	item.RateLimitMode = pool.RateLimitMode

	tokenPolicies, err := deps.loadPolicies(pool.Id, model.PoolQuotaScopeToken)
	if err != nil {
		return nil, err
	}
	validTokenPolicies, maxWindowSeconds := filterValidRequestCountPolicies(tokenPolicies)
	limitByWindowSeconds := make(map[int]int64, len(validTokenPolicies))
	for _, policy := range validTokenPolicies {
		if policy == nil || policy.WindowSeconds <= 0 || policy.LimitCount <= 0 {
			continue
		}
		limit := int64(policy.LimitCount)
		// If duplicate windows exist, keep the strictest (smallest) limit.
		if old, ok := limitByWindowSeconds[policy.WindowSeconds]; !ok || limit < old {
			limitByWindowSeconds[policy.WindowSeconds] = limit
		}
	}
	if len(validTokenPolicies) == 0 || maxWindowSeconds <= 0 {
		userPolicies, userErr := deps.loadPolicies(pool.Id, model.PoolQuotaScopeUser)
		if userErr != nil {
			return nil, userErr
		}
		validUserPolicies, _ := filterValidRequestCountPolicies(userPolicies)
		item.ScopeType = model.PoolQuotaScopeToken
		if len(validUserPolicies) > 0 {
			item.ScopeType = model.PoolQuotaScopeUser
			return buildUnavailableTokenPoolUsage(item, windows, windowSeconds, tokenPoolUsageReasonUserScopeOnly), nil
		}
		return buildUnavailableTokenPoolUsage(item, windows, windowSeconds, tokenPoolUsageReasonTokenScopeDisabled), nil
	}

	item.ScopeType = model.PoolQuotaScopeToken
	item.TokenScopeEnabled = true
	item.RetentionWindowSeconds = maxWindowSeconds

	for _, window := range windows {
		result := &TokenPoolUsageWindow{
			Window:        window,
			WindowSeconds: windowSeconds[window],
		}
		if limit, ok := limitByWindowSeconds[result.WindowSeconds]; ok {
			v := limit
			result.LimitCount = &v
		}
		if result.WindowSeconds > maxWindowSeconds {
			result.Available = false
			result.Reason = tokenPoolUsageReasonWindowNotRetained
			item.Usage[window] = result
			continue
		}
		var (
			count    int64
			countErr error
		)
		if pool.RateLimitMode == model.PoolRateLimitModeFixed && common.PoolFixedWindowEnabled && deps.countFixedRequestsByToken != nil {
			count, countErr = deps.countFixedRequestsByToken(token.Id, result.WindowSeconds)
		} else {
			count, countErr = deps.countRequestsByToken(token.Id, result.WindowSeconds)
		}
		if countErr != nil {
			return nil, countErr
		}
		result.Available = true
		result.Count = &count
		// Populate reset time fields for fixed-window pools
		if pool.RateLimitMode == model.PoolRateLimitModeFixed {
			nowUnix := time.Now().Unix()
			resetAt := model.FixedWindowResetAt(nowUnix, result.WindowSeconds)
			resetIn := model.FixedWindowResetInSeconds(nowUnix, result.WindowSeconds)
			result.ResetAt = &resetAt
			result.ResetInSeconds = &resetIn
		}
		item.Usage[window] = result
	}
	return item, nil
}

func GetTokenPoolUsageBatch(c *gin.Context) {
	userId := c.GetInt("id")
	if userId <= 0 {
		common.ApiErrorMsg(c, "invalid user")
		return
	}

	req := TokenPoolUsageBatchRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	windows, windowSeconds, err := normalizeTokenPoolUsageWindows(req.Windows)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	uniqueIds := make([]int, 0, len(req.Ids))
	seen := make(map[int]struct{}, len(req.Ids))
	for _, id := range req.Ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIds = append(uniqueIds, id)
	}
	if len(uniqueIds) == 0 {
		common.ApiSuccess(c, gin.H{"items": []*TokenPoolUsageItem{}})
		return
	}

	tokens, err := model.GetUserTokensByIds(userId, uniqueIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tokenById := make(map[int]*model.Token, len(tokens))
	for _, token := range tokens {
		if token == nil {
			continue
		}
		tokenById[token.Id] = token
	}

	items := make([]*TokenPoolUsageItem, 0, len(uniqueIds))
	for _, id := range uniqueIds {
		token := tokenById[id]
		if token == nil {
			continue
		}
		item, buildErr := buildTokenPoolUsageItem(token, windows, windowSeconds)
		if buildErr != nil {
			common.ApiError(c, buildErr)
			return
		}
		llmUsage, llmErr := buildTokenPoolLLMTokenUsage(token, windows, windowSeconds)
		if llmErr != nil {
			common.ApiError(c, llmErr)
			return
		}
		item.LlmTokenUsage = llmUsage
		items = append(items, item)
	}

	common.ApiSuccess(c, gin.H{
		"items":       items,
		"windows":     windows,
		"data_source": tokenPoolUsageDataSource,
	})
}

func GetTokenPoolUsageSelf(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId <= 0 {
		common.ApiErrorMsg(c, "invalid token")
		return
	}

	windows, windowSeconds, err := normalizeTokenPoolUsageWindows(nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	token, err := model.GetTokenById(tokenId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	item, err := buildTokenPoolUsageItemFunc(token, windows, windowSeconds)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	llmUsage, err := buildTokenPoolLLMTokenUsage(token, windows, windowSeconds)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	resolved, resErr := model.ResolvePoolForContext(token.UserId, token.Id, token.Group)
	if resErr != nil && !errors.Is(resErr, gorm.ErrRecordNotFound) {
		common.ApiError(c, resErr)
		return
	}
	applyTokenPoolUsagePoolName(item, resolved)
	payload := gin.H{
		"item":            item,
		"windows":         windows,
		"data_source":     tokenPoolUsageDataSource,
		"llm_token_usage": llmUsage,
	}
	if resolved != nil && resolved.Id > 0 {
		displayPoolName := poolDisplayName(resolved)
		payload["resolved_pool_id"] = resolved.Id
		payload["resolved_pool_name"] = displayPoolName
		payload["resolved_pool"] = buildTokenPoolResolvedPoolSummary(resolved)
		subInfo, subErr := buildTokenPoolSubscriptionInfo(c.Request.Context(), token, resolved)
		if subErr != nil {
			common.ApiError(c, subErr)
			return
		}
		if subInfo != nil {
			payload["pool_subscription"] = subInfo
		}
	}

	common.ApiSuccess(c, payload)
}

func resetTokenFixedPoolUsageWithDeps(token *model.Token, window string, adminUserId int, deps tokenPoolUsageResetDeps) error {
	if token == nil || token.Id <= 0 {
		return errors.New("invalid token")
	}
	windows, windowSeconds, err := normalizeTokenPoolUsageWindows([]string{window})
	if err != nil {
		return err
	}
	if len(windows) != 1 {
		return errors.New("exactly one window is required")
	}
	window = windows[0]
	windowSec := windowSeconds[window]

	pool, err := deps.resolvePool(token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("no resolved pool for token")
		}
		return err
	}
	if pool == nil {
		return errors.New("no resolved pool for token")
	}
	if pool.RateLimitMode != model.PoolRateLimitModeFixed {
		return errors.New("pool is not in fixed-window mode")
	}
	if !common.PoolFixedWindowEnabled {
		return errors.New("fixed-window pool usage is disabled")
	}

	tokenPolicies, err := deps.loadPolicies(pool.Id, model.PoolQuotaScopeToken)
	if err != nil {
		return err
	}
	validTokenPolicies, maxWindowSeconds := filterValidRequestCountPolicies(tokenPolicies)
	if len(validTokenPolicies) == 0 || maxWindowSeconds <= 0 {
		userPolicies, userErr := deps.loadPolicies(pool.Id, model.PoolQuotaScopeUser)
		if userErr != nil {
			return userErr
		}
		validUserPolicies, _ := filterValidRequestCountPolicies(userPolicies)
		if len(validUserPolicies) > 0 {
			return errors.New("token scope policies are not enabled for this pool")
		}
		return errors.New("token scope policies are not enabled for this pool")
	}
	if windowSec > maxWindowSeconds {
		return errors.New("window is not retained by this pool")
	}
	hasWindowPolicy := false
	for _, policy := range validTokenPolicies {
		if policy != nil && policy.WindowSeconds == windowSec {
			hasWindowPolicy = true
			break
		}
	}
	if !hasWindowPolicy {
		return errors.New("window is not configured for this pool")
	}

	scopeKey := "token:" + strconv.Itoa(token.Id)
	nowUnix := time.Now().Unix()
	if err := deps.resetFixedWindow(context.Background(), scopeKey, windowSec, nowUnix); err != nil {
		return err
	}
	if deps.recordManageLog != nil && adminUserId > 0 {
		deps.recordManageLog(adminUserId, fmt.Sprintf(
			"reset fixed pool request count for token %d (%s) window %s",
			token.Id,
			strings.TrimSpace(token.Name),
			window,
		))
	}
	return nil
}

func ResetTokenFixedPoolUsage(c *gin.Context) {
	adminUserId := c.GetInt("id")
	if adminUserId <= 0 {
		common.ApiErrorMsg(c, "invalid user")
		return
	}

	tokenId, err := strconv.Atoi(c.Param("id"))
	if err != nil || tokenId <= 0 {
		common.ApiError(c, err)
		return
	}

	req := TokenPoolUsageResetRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(req.Window) == "" {
		common.ApiErrorMsg(c, "window is required")
		return
	}

	token, err := model.GetTokenById(tokenId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	deps := tokenPoolUsageResetDeps{
		resolvePool: func(token *model.Token) (*model.Pool, error) {
			return model.ResolvePoolForContext(token.UserId, token.Id, token.Group)
		},
		loadPolicies: func(poolId int, scopeType string) ([]*model.PoolQuotaPolicy, error) {
			return model.GetPoolQuotaPolicies(poolId, model.PoolQuotaMetricRequestCount, scopeType)
		},
		resetFixedWindow: model.ResetFixedWindowCount,
		recordManageLog: func(userId int, content string) {
			model.RecordLog(userId, model.LogTypeManage, content)
		},
	}
	if err := resetTokenFixedPoolUsageWithDeps(token, req.Window, adminUserId, deps); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	common.ApiSuccess(c, gin.H{
		"token_id": tokenId,
		"window":   strings.TrimSpace(req.Window),
	})
}
