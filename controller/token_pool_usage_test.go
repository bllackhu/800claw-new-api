package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuildTokenPoolUsageItem_NoResolvedPool(t *testing.T) {
	token := &model.Token{Id: 11, UserId: 101, Name: "token-a", Group: "g-test"}
	windows, windowSeconds, err := normalizeTokenPoolUsageWindows([]string{"5h", "7d"})
	require.NoError(t, err)

	item, err := buildTokenPoolUsageItemWithDeps(token, windows, windowSeconds, tokenPoolUsageBuilderDeps{
		resolvePool: func(token *model.Token) (*model.Pool, error) {
			return nil, nil
		},
		loadPolicies: func(poolId int, scopeType string) ([]*model.PoolQuotaPolicy, error) {
			return nil, nil
		},
		isRedisReady: func() bool { return false },
		countByWindow: func(redisKey string, windowSeconds int) (int64, error) {
			return 0, nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, token.Id, item.TokenId)
	require.Empty(t, item.PoolName)
	require.False(t, item.Usage["5h"].Available)
	require.Equal(t, tokenPoolUsageReasonNoResolvedPool, item.Usage["5h"].Reason)
	require.False(t, item.Usage["7d"].Available)
	require.Equal(t, tokenPoolUsageReasonNoResolvedPool, item.Usage["7d"].Reason)
}

func TestBuildTokenPoolUsageItem_UserScopeOnly(t *testing.T) {
	token := &model.Token{Id: 12, UserId: 102, Name: "token-b", Group: "g-test"}
	pool := &model.Pool{Id: 21, Name: "user_scope_only_pool"}
	windows, windowSeconds, err := normalizeTokenPoolUsageWindows([]string{"5h"})
	require.NoError(t, err)

	item, err := buildTokenPoolUsageItemWithDeps(token, windows, windowSeconds, tokenPoolUsageBuilderDeps{
		resolvePool: func(token *model.Token) (*model.Pool, error) {
			return pool, nil
		},
		loadPolicies: func(poolId int, scopeType string) ([]*model.PoolQuotaPolicy, error) {
			if scopeType == model.PoolQuotaScopeUser {
				return []*model.PoolQuotaPolicy{
					{
						Metric:        model.PoolQuotaMetricRequestCount,
						ScopeType:     model.PoolQuotaScopeUser,
						WindowSeconds: 7 * 24 * 3600,
						LimitCount:    10,
						Enabled:       true,
					},
				}, nil
			}
			return nil, nil
		},
		isRedisReady: func() bool { return false },
		countByWindow: func(redisKey string, windowSeconds int) (int64, error) {
			return 0, nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, pool.Id, item.PoolId)
	require.Equal(t, model.PoolQuotaScopeUser, item.ScopeType)
	require.False(t, item.TokenScopeEnabled)
	require.False(t, item.Usage["5h"].Available)
	require.Equal(t, tokenPoolUsageReasonUserScopeOnly, item.Usage["5h"].Reason)
}

func TestBuildTokenPoolUsageItem_WindowNotRetained(t *testing.T) {
	token := &model.Token{Id: 13, UserId: 103, Name: "token-c", Group: "g-test"}
	pool := &model.Pool{Id: 22, Name: "retention_pool"}
	windows, windowSeconds, err := normalizeTokenPoolUsageWindows([]string{"30d"})
	require.NoError(t, err)

	item, err := buildTokenPoolUsageItemWithDeps(token, windows, windowSeconds, tokenPoolUsageBuilderDeps{
		resolvePool: func(token *model.Token) (*model.Pool, error) {
			return pool, nil
		},
		loadPolicies: func(poolId int, scopeType string) ([]*model.PoolQuotaPolicy, error) {
			if scopeType == model.PoolQuotaScopeToken {
				return []*model.PoolQuotaPolicy{
					{
						Metric:        model.PoolQuotaMetricRequestCount,
						ScopeType:     model.PoolQuotaScopeToken,
						WindowSeconds: 5 * 3600,
						LimitCount:    10,
						Enabled:       true,
					},
				}, nil
			}
			return nil, nil
		},
		isRedisReady: func() bool { return true },
		countByWindow: func(redisKey string, windowSeconds int) (int64, error) {
			return 0, nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, pool.Id, item.PoolId)
	require.True(t, item.TokenScopeEnabled)
	require.Equal(t, 5*3600, item.RetentionWindowSeconds)
	require.False(t, item.Usage["30d"].Available)
	require.Equal(t, tokenPoolUsageReasonWindowNotRetained, item.Usage["30d"].Reason)
	require.Nil(t, item.Usage["30d"].LimitCount)
}

func TestBuildTokenPoolUsageItem_IncludeWindowLimitCount(t *testing.T) {
	token := &model.Token{Id: 14, UserId: 104, Name: "token-d", Group: "g-test"}
	pool := &model.Pool{Id: 23, Name: "limit_pool"}
	windows, windowSeconds, err := normalizeTokenPoolUsageWindows([]string{"5h", "7d"})
	require.NoError(t, err)

	item, err := buildTokenPoolUsageItemWithDeps(token, windows, windowSeconds, tokenPoolUsageBuilderDeps{
		resolvePool: func(token *model.Token) (*model.Pool, error) {
			return pool, nil
		},
		loadPolicies: func(poolId int, scopeType string) ([]*model.PoolQuotaPolicy, error) {
			if scopeType != model.PoolQuotaScopeToken {
				return nil, nil
			}
			return []*model.PoolQuotaPolicy{
				{
					Metric:        model.PoolQuotaMetricRequestCount,
					ScopeType:     model.PoolQuotaScopeToken,
					WindowSeconds: 5 * 3600,
					LimitCount:    300,
					Enabled:       true,
				},
				{
					Metric:        model.PoolQuotaMetricRequestCount,
					ScopeType:     model.PoolQuotaScopeToken,
					WindowSeconds: 7 * 24 * 3600,
					LimitCount:    3000,
					Enabled:       true,
				},
			}, nil
		},
		isRedisReady: func() bool { return true },
		countByWindow: func(redisKey string, windowSeconds int) (int64, error) {
			if windowSeconds == 5*3600 {
				return 4, nil
			}
			return 16, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, item.Usage["5h"])
	require.NotNil(t, item.Usage["7d"])
	require.NotNil(t, item.Usage["5h"].Count)
	require.NotNil(t, item.Usage["5h"].LimitCount)
	require.NotNil(t, item.Usage["7d"].LimitCount)
	require.EqualValues(t, 4, *item.Usage["5h"].Count)
	require.EqualValues(t, 300, *item.Usage["5h"].LimitCount)
	require.EqualValues(t, 3000, *item.Usage["7d"].LimitCount)
}

func TestNormalizeTokenPoolUsageWindows_Defaults(t *testing.T) {
	windows, windowSeconds, err := normalizeTokenPoolUsageWindows(nil)
	require.NoError(t, err)
	require.Equal(t, []string{"5h", "7d", "30d"}, windows)
	require.Equal(t, 5*3600, windowSeconds["5h"])
	require.Equal(t, 7*24*3600, windowSeconds["7d"])
	require.Equal(t, 30*24*3600, windowSeconds["30d"])
}

func TestGetTokenPoolUsageSelf_DefaultWindows(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	migratePoolTablesForTokenUsageTests(t, db)
	token := seedToken(t, db, 88, "agent-token", "agent-token-key")
	pool := &model.Pool{Name: "resolved-for-usage-self", Status: model.PoolStatusEnabled}
	require.NoError(t, db.Create(pool).Error)
	require.NoError(t, db.Create(&model.PoolBinding{
		BindingType:  model.PoolBindingTypeToken,
		BindingValue: strconv.Itoa(token.Id),
		PoolId:       pool.Id,
		Enabled:      true,
	}).Error)

	originalBuilder := buildTokenPoolUsageItemFunc
	t.Cleanup(func() {
		buildTokenPoolUsageItemFunc = originalBuilder
	})
	buildTokenPoolUsageItemFunc = func(token *model.Token, windows []string, windowSeconds map[string]int) (*TokenPoolUsageItem, error) {
		require.Equal(t, []string{"5h", "7d", "30d"}, windows)
		require.Equal(t, 5*3600, windowSeconds["5h"])
		require.Equal(t, 7*24*3600, windowSeconds["7d"])
		require.Equal(t, 30*24*3600, windowSeconds["30d"])
		return &TokenPoolUsageItem{
			TokenId:    token.Id,
			DataSource: tokenPoolUsageDataSource,
			Usage: map[string]*TokenPoolUsageWindow{
				"5h": {
					Window:        "5h",
					WindowSeconds: windowSeconds["5h"],
					Available:     true,
					Count:         countPtr(3),
				},
			},
		}, nil
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/usage/token/pool", nil)
	ctx.Set("token_id", token.Id)
	ctx.Set("id", token.UserId)

	GetTokenPoolUsageSelf(ctx)

	var response tokenAPIResponse
	err := common.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)
	require.True(t, response.Success)
	require.Empty(t, response.Message)

	var payload struct {
		Item             TokenPoolUsageItem `json:"item"`
		ResolvedPoolId   int                `json:"resolved_pool_id"`
		ResolvedPoolName string             `json:"resolved_pool_name"`
		Windows          []string           `json:"windows"`
		DataSource       string             `json:"data_source"`
		LlmTokenUsage struct {
			DataSource string                              `json:"data_source"`
			ByWindow   map[string]*TokenPoolLLMTokenWindow `json:"by_window"`
			Lifetime   struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
				TotalTokens      int64 `json:"total_tokens"`
			} `json:"lifetime"`
		} `json:"llm_token_usage"`
	}
	err = common.Unmarshal(response.Data, &payload)
	require.NoError(t, err)
	require.Equal(t, []string{"5h", "7d", "30d"}, payload.Windows)
	require.Equal(t, tokenPoolUsageDataSource, payload.DataSource)
	require.Equal(t, token.Id, payload.Item.TokenId)
	require.Equal(t, pool.Id, payload.ResolvedPoolId)
	require.Equal(t, pool.Name, payload.ResolvedPoolName)
	require.Contains(t, payload.Item.Usage, "5h")
	require.NotNil(t, payload.Item.Usage["5h"].Count)
	require.EqualValues(t, 3, *payload.Item.Usage["5h"].Count)
	require.Equal(t, tokenPoolLLMTokenDataSource, payload.LlmTokenUsage.DataSource)
	require.NotNil(t, payload.LlmTokenUsage.ByWindow["5h"])
	require.EqualValues(t, 0, payload.LlmTokenUsage.ByWindow["5h"].PromptTokens)
}

func TestGetTokenPoolUsageSelf_LLMTokenAggregates(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	migratePoolTablesForTokenUsageTests(t, db)
	token := seedToken(t, db, 91, "tok-llm", "tok-llm-key")

	now := time.Now().Unix()
	bRecent := model.AlignTokenLLMUsageBucketStart(now - 3600)
	bOlder := model.AlignTokenLLMUsageBucketStart(now - 3*24*3600)
	require.NoError(t, db.Create(&model.TokenLLMUsageBucket{
		TokenId:          token.Id,
		BucketStart:      bRecent,
		PromptTokens:     100,
		CompletionTokens: 40,
		RequestCount:     1,
	}).Error)
	require.NoError(t, db.Create(&model.TokenLLMUsageBucket{
		TokenId:          token.Id,
		BucketStart:      bOlder,
		PromptTokens:     1,
		CompletionTokens: 2,
		RequestCount:     1,
	}).Error)
	require.NoError(t, db.Model(token).Updates(map[string]interface{}{
		"llm_prompt_tokens_total":     1101,
		"llm_completion_tokens_total": 542,
	}).Error)

	originalBuilder := buildTokenPoolUsageItemFunc
	t.Cleanup(func() {
		buildTokenPoolUsageItemFunc = originalBuilder
	})
	buildTokenPoolUsageItemFunc = func(token *model.Token, windows []string, windowSeconds map[string]int) (*TokenPoolUsageItem, error) {
		return &TokenPoolUsageItem{
			TokenId:    token.Id,
			DataSource: tokenPoolUsageDataSource,
			Usage:      map[string]*TokenPoolUsageWindow{},
		}, nil
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/usage/token/pool", nil)
	ctx.Set("token_id", token.Id)
	ctx.Set("id", token.UserId)

	GetTokenPoolUsageSelf(ctx)

	var response tokenAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)

	var payload struct {
		LlmTokenUsage struct {
			DataSource string                              `json:"data_source"`
			ByWindow   map[string]*TokenPoolLLMTokenWindow `json:"by_window"`
			Lifetime   struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
				TotalTokens      int64 `json:"total_tokens"`
			} `json:"lifetime"`
		} `json:"llm_token_usage"`
	}
	require.NoError(t, common.Unmarshal(response.Data, &payload))
	require.Equal(t, tokenPoolLLMTokenDataSource, payload.LlmTokenUsage.DataSource)

	w5 := payload.LlmTokenUsage.ByWindow["5h"]
	require.NotNil(t, w5)
	require.EqualValues(t, 100, w5.PromptTokens)
	require.EqualValues(t, 40, w5.CompletionTokens)
	require.EqualValues(t, 140, w5.TotalTokens)

	w7 := payload.LlmTokenUsage.ByWindow["7d"]
	require.NotNil(t, w7)
	require.EqualValues(t, 101, w7.PromptTokens)
	require.EqualValues(t, 42, w7.CompletionTokens)
	require.EqualValues(t, 143, w7.TotalTokens)

	require.EqualValues(t, 1101, payload.LlmTokenUsage.Lifetime.PromptTokens)
	require.EqualValues(t, 542, payload.LlmTokenUsage.Lifetime.CompletionTokens)
	require.EqualValues(t, 1643, payload.LlmTokenUsage.Lifetime.TotalTokens)
}

func countPtr(v int64) *int64 {
	return &v
}

func migratePoolTablesForTokenUsageTests(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(
		&model.Pool{},
		&model.PoolBinding{},
		&model.TokenPoolSubscription{},
		&model.TokenPoolSubscriptionOrder{},
	))
}

func TestGetTokenPoolUsageSelf_EmptyPoolNameFallback(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	migratePoolTablesForTokenUsageTests(t, db)
	token := seedToken(t, db, 92, "", "unnamed-token-key-1234")
	pool := &model.Pool{Name: "", Description: "desc", Status: model.PoolStatusEnabled, MonthlyPriceCny: 25}
	require.NoError(t, db.Create(pool).Error)
	require.NoError(t, db.Create(&model.PoolBinding{
		BindingType:  model.PoolBindingTypeToken,
		BindingValue: strconv.Itoa(token.Id),
		PoolId:       pool.Id,
		Enabled:      true,
	}).Error)

	originalBuilder := buildTokenPoolUsageItemFunc
	t.Cleanup(func() {
		buildTokenPoolUsageItemFunc = originalBuilder
	})
	buildTokenPoolUsageItemFunc = func(token *model.Token, windows []string, windowSeconds map[string]int) (*TokenPoolUsageItem, error) {
		return &TokenPoolUsageItem{
			TokenId:    token.Id,
			PoolId:     pool.Id,
			DataSource: tokenPoolUsageDataSource,
			Usage:      map[string]*TokenPoolUsageWindow{},
		}, nil
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/usage/token/pool", nil)
	ctx.Set("token_id", token.Id)
	ctx.Set("id", token.UserId)

	GetTokenPoolUsageSelf(ctx)

	var response tokenAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)

	var payload struct {
		ResolvedPoolName string                       `json:"resolved_pool_name"`
		Item             TokenPoolUsageItem           `json:"item"`
		ResolvedPool     TokenPoolResolvedPoolSummary `json:"resolved_pool"`
		PoolSubscription TokenPoolSubscriptionInfo    `json:"pool_subscription"`
	}
	require.NoError(t, common.Unmarshal(response.Data, &payload))
	require.Equal(t, poolDisplayNameFallback, payload.ResolvedPoolName)
	require.Equal(t, poolDisplayNameFallback, payload.ResolvedPool.Name)
	require.Equal(t, "desc", payload.ResolvedPool.Description)
	require.True(t, payload.PoolSubscription.CheckoutAvailable)
	require.EqualValues(t, 25, payload.PoolSubscription.MonthlyPriceCny)
	require.False(t, payload.PoolSubscription.Active)
}

func TestGetTokenPoolUsageSelf_ActiveSubscription(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	migratePoolTablesForTokenUsageTests(t, db)
	token := seedToken(t, db, 93, "sub-tok", "sub-token-key-abcdef")
	pool := &model.Pool{Name: "Pro pool", Status: model.PoolStatusEnabled, MonthlyPriceCny: 40}
	require.NoError(t, db.Create(pool).Error)
	require.NoError(t, db.Create(&model.PoolBinding{
		BindingType:  model.PoolBindingTypeToken,
		BindingValue: strconv.Itoa(token.Id),
		PoolId:       pool.Id,
		Enabled:      true,
	}).Error)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.TokenPoolSubscription{
		TokenId:     token.Id,
		PoolId:      pool.Id,
		PeriodStart: now - 3600,
		PeriodEnd:   now + 7*24*3600,
	}).Error)

	originalBuilder := buildTokenPoolUsageItemFunc
	t.Cleanup(func() {
		buildTokenPoolUsageItemFunc = originalBuilder
	})
	buildTokenPoolUsageItemFunc = func(token *model.Token, windows []string, windowSeconds map[string]int) (*TokenPoolUsageItem, error) {
		return &TokenPoolUsageItem{TokenId: token.Id, DataSource: tokenPoolUsageDataSource, Usage: map[string]*TokenPoolUsageWindow{}}, nil
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/usage/token/pool", nil)
	ctx.Set("token_id", token.Id)
	ctx.Set("id", token.UserId)

	GetTokenPoolUsageSelf(ctx)

	var response tokenAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)

	var payload struct {
		PoolSubscription TokenPoolSubscriptionInfo `json:"pool_subscription"`
	}
	require.NoError(t, common.Unmarshal(response.Data, &payload))
	require.True(t, payload.PoolSubscription.Active)
	require.Greater(t, payload.PoolSubscription.PeriodEnd, now)
}
