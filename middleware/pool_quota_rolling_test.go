package middleware

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildPoolQuotaTestCtx(userId, tokenId int, presetScopeKey string) *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	if userId > 0 {
		common.SetContextKey(ctx, constant.ContextKeyUserId, userId)
	}
	if tokenId > 0 {
		common.SetContextKey(ctx, constant.ContextKeyTokenId, tokenId)
	}
	if presetScopeKey != "" {
		common.SetContextKey(ctx, constant.ContextKeyPoolScopeKey, presetScopeKey)
	}
	return ctx
}

func buildPolicies(scope string, windowsAndLimits ...int) []*model.PoolQuotaPolicy {
	items := make([]*model.PoolQuotaPolicy, 0, len(windowsAndLimits)/2)
	for i := 0; i+1 < len(windowsAndLimits); i += 2 {
		items = append(items, &model.PoolQuotaPolicy{
			Metric:        model.PoolQuotaMetricRequestCount,
			ScopeType:     scope,
			WindowSeconds: windowsAndLimits[i],
			LimitCount:    windowsAndLimits[i+1],
		})
	}
	return items
}

func TestLoadPoolQuotaScopePoliciesAndScopeKey_TokenOnly(t *testing.T) {
	t.Parallel()
	ctx := buildPoolQuotaTestCtx(1, 9, "user:1")
	loader := func(_ int, _ string, scope string) ([]*model.PoolQuotaPolicy, error) {
		if scope == model.PoolQuotaScopeToken {
			return buildPolicies(model.PoolQuotaScopeToken, 300, 2, 18000, 1000), nil
		}
		return nil, nil
	}

	policies, scopeKey, maxWindow, err := loadPoolQuotaScopePoliciesAndScopeKey(ctx, 1, loader)
	require.NoError(t, err)
	require.Len(t, policies, 2)
	assert.Equal(t, "token:9", scopeKey)
	assert.Equal(t, 18000, maxWindow)
	assert.Equal(t, "token:9", common.GetContextKeyString(ctx, constant.ContextKeyPoolScopeKey))
}

func TestLoadPoolQuotaScopePoliciesAndScopeKey_UserOnly(t *testing.T) {
	t.Parallel()
	ctx := buildPoolQuotaTestCtx(1, 0, "user:1")
	loader := func(_ int, _ string, scope string) ([]*model.PoolQuotaPolicy, error) {
		if scope == model.PoolQuotaScopeUser {
			return buildPolicies(model.PoolQuotaScopeUser, 300, 2, 18000, 1000), nil
		}
		return nil, nil
	}

	policies, scopeKey, maxWindow, err := loadPoolQuotaScopePoliciesAndScopeKey(ctx, 1, loader)
	require.NoError(t, err)
	require.Len(t, policies, 2)
	assert.Equal(t, "user:1", scopeKey)
	assert.Equal(t, 18000, maxWindow)
}

func TestLoadPoolQuotaScopePoliciesAndScopeKey_BothScopeTokenPrecedence(t *testing.T) {
	t.Parallel()
	ctx := buildPoolQuotaTestCtx(1, 12, "user:1")
	loader := func(_ int, _ string, scope string) ([]*model.PoolQuotaPolicy, error) {
		if scope == model.PoolQuotaScopeToken {
			return buildPolicies(model.PoolQuotaScopeToken, 60, 1), nil
		}
		if scope == model.PoolQuotaScopeUser {
			return buildPolicies(model.PoolQuotaScopeUser, 300, 2), nil
		}
		return nil, nil
	}

	policies, scopeKey, maxWindow, err := loadPoolQuotaScopePoliciesAndScopeKey(ctx, 1, loader)
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, model.PoolQuotaScopeToken, policies[0].ScopeType)
	assert.Equal(t, "token:12", scopeKey)
	assert.Equal(t, 60, maxWindow)
}

func TestLoadPoolQuotaScopePoliciesAndScopeKey_TokenMissingFallbackUser(t *testing.T) {
	t.Parallel()
	ctx := buildPoolQuotaTestCtx(1, 0, "user:1")
	loader := func(_ int, _ string, scope string) ([]*model.PoolQuotaPolicy, error) {
		if scope == model.PoolQuotaScopeToken {
			return buildPolicies(model.PoolQuotaScopeToken, 300, 2), nil
		}
		return nil, nil
	}

	policies, scopeKey, maxWindow, err := loadPoolQuotaScopePoliciesAndScopeKey(ctx, 1, loader)
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, "user:1", scopeKey)
	assert.Equal(t, 300, maxWindow)
	assert.Equal(t, "user:1", common.GetContextKeyString(ctx, constant.ContextKeyPoolScopeKey))
}

func TestLoadPoolQuotaScopePoliciesAndScopeKey_TokenIsolationByTokenId(t *testing.T) {
	t.Parallel()
	loader := func(_ int, _ string, scope string) ([]*model.PoolQuotaPolicy, error) {
		if scope == model.PoolQuotaScopeToken {
			return buildPolicies(model.PoolQuotaScopeToken, 300, 2), nil
		}
		return nil, nil
	}

	ctxA := buildPoolQuotaTestCtx(1, 101, "")
	_, scopeKeyA, _, err := loadPoolQuotaScopePoliciesAndScopeKey(ctxA, 1, loader)
	require.NoError(t, err)

	ctxB := buildPoolQuotaTestCtx(1, 102, "")
	_, scopeKeyB, _, err := loadPoolQuotaScopePoliciesAndScopeKey(ctxB, 1, loader)
	require.NoError(t, err)

	assert.Equal(t, "token:101", scopeKeyA)
	assert.Equal(t, "token:102", scopeKeyB)
	assert.NotEqual(t, scopeKeyA, scopeKeyB)
}

func TestPoolRollingRequestRedisKey_TokenScopeUsesTokenKey(t *testing.T) {
	t.Parallel()
	key := model.PoolRollingRequestRedisKey(99, "token:12")
	assert.Equal(t, model.TokenRollingRequestRedisKey(12), key)
	assert.NotContains(t, key, "pool:rq:events:99")
}

func TestPoolRollingRequestRedisKey_UserScopeStaysPoolScoped(t *testing.T) {
	t.Parallel()
	key := model.PoolRollingRequestRedisKey(99, "user:12")
	assert.Equal(t, "pool:rq:events:99:user:12", key)
}

func TestLoadPoolQuotaScopePoliciesAndScopeKey_LoaderError(t *testing.T) {
	t.Parallel()
	ctx := buildPoolQuotaTestCtx(1, 0, "")
	loader := func(_ int, _ string, scope string) ([]*model.PoolQuotaPolicy, error) {
		if scope == model.PoolQuotaScopeToken {
			return nil, errors.New("boom")
		}
		return nil, nil
	}

	policies, scopeKey, maxWindow, err := loadPoolQuotaScopePoliciesAndScopeKey(ctx, 1, loader)
	require.Error(t, err)
	assert.Nil(t, policies)
	assert.Equal(t, "", scopeKey)
	assert.Equal(t, 0, maxWindow)
}

func TestFixedWindowCounterKey(t *testing.T) {
	t.Parallel()
	// At Unix 1750000000, a 5h (18000s) window: bucket = 1750000000 / 18000 = 97222
	key := model.FixedWindowCounterKey("token:123", 18000, 1750000000)
	assert.Equal(t, "token:rq:fixed:token:123:18000:97222", key)

	// At Unix 1750018000 (5h later), bucket = 1750018000 / 18000 = 97223
	key2 := model.FixedWindowCounterKey("token:123", 18000, 1750018000)
	assert.Equal(t, "token:rq:fixed:token:123:18000:97223", key2)
	assert.NotEqual(t, key, key2, "keys should differ across bucket boundaries")
}

func TestFixedWindowBucketNumber(t *testing.T) {
	t.Parallel()
	// 5h window
	bucket := model.FixedWindowBucketNumber(1750000000, 18000)
	assert.Equal(t, int64(97222), bucket)

	// 7d window
	bucket7d := model.FixedWindowBucketNumber(1750000000, 604800)
	assert.Equal(t, int64(2893), bucket7d)

	// 30d window
	bucket30d := model.FixedWindowBucketNumber(1750000000, 2592000)
	assert.Equal(t, int64(675), bucket30d)
}

func TestFixedWindowResetAt(t *testing.T) {
	t.Parallel()
	// At Unix 1750000000, 5h window: reset at (97222+1)*18000 = 1750014000
	resetAt := model.FixedWindowResetAt(1750000000, 18000)
	assert.Equal(t, int64(1750014000), resetAt)

	// At Unix 1750014000 (exactly on boundary), 5h window: bucket = 1750014000/18000 = 97223, reset at (97223+1)*18000 = 1750032000
	resetAt2 := model.FixedWindowResetAt(1750014000, 18000)
	assert.Equal(t, int64(1750032000), resetAt2)

	// 7d window
	resetAt7d := model.FixedWindowResetAt(1750000000, 604800)
	assert.Equal(t, int64(1750604800), resetAt7d)
}

func TestFixedWindowResetInSeconds(t *testing.T) {
	t.Parallel()
	// At Unix 1750000000, 5h window: reset at 1750014000, so resetIn = 14000s
	resetIn := model.FixedWindowResetInSeconds(1750000000, 18000)
	assert.Equal(t, int64(14000), resetIn)

	// 30d window: resetIn = 2592000s
	resetIn30d := model.FixedWindowResetInSeconds(1750000000, 2592000)
	assert.Equal(t, int64(2592000), resetIn30d)
}

func TestFilterValidPoolQuotaPolicies(t *testing.T) {
	t.Parallel()
	policies := []*model.PoolQuotaPolicy{
		{Metric: model.PoolQuotaMetricRequestCount, ScopeType: model.PoolQuotaScopeToken, WindowSeconds: 18000, LimitCount: 100},
		{Metric: model.PoolQuotaMetricRequestCount, ScopeType: model.PoolQuotaScopeToken, WindowSeconds: 0, LimitCount: 10},   // invalid: zero window
		{Metric: model.PoolQuotaMetricRequestCount, ScopeType: model.PoolQuotaScopeToken, WindowSeconds: 300, LimitCount: 0},  // invalid: zero limit
		{Metric: model.PoolQuotaMetricRequestCount, ScopeType: model.PoolQuotaScopeToken, WindowSeconds: 604800, LimitCount: 1000},
		{Metric: model.PoolQuotaMetricRequestCount, ScopeType: model.PoolQuotaScopeToken, WindowSeconds: 2592000, LimitCount: 5000},
		nil, // nil pointer
	}

	valid, maxWindow := filterValidPoolQuotaPolicies(policies)
	require.Len(t, valid, 3)
	assert.Equal(t, 2592000, maxWindow)
	assert.Equal(t, 18000, valid[0].WindowSeconds)
	assert.Equal(t, 604800, valid[1].WindowSeconds)
	assert.Equal(t, 2592000, valid[2].WindowSeconds)
}

