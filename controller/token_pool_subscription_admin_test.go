package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPutTokenPoolSubscription_AdminUpsert(t *testing.T) {
	setupTokenPoolSubscriptionCheckoutTestDB(t)

	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.Token{
		Id: 7, UserId: 1, Name: "tok7", Key: "sk-test-token-7-abcdefghijklmnopqrstuv",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Pool{
		Id: 30, Name: "pool-30", Status: model.PoolStatusEnabled,
	}).Error)

	future := now + 7*24*3600
	body, _ := json.Marshal(map[string]interface{}{
		"token_id":   7,
		"pool_id":    30,
		"period_end": future,
	})
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/pool/token_subscription", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	PutTokenPoolSubscription(ctx)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			TokenId   int   `json:"token_id"`
			PoolId    int   `json:"pool_id"`
			PeriodEnd int64 `json:"period_end"`
			Active    bool  `json:"active"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, 7, resp.Data.TokenId)
	require.Equal(t, future, resp.Data.PeriodEnd)
	require.True(t, resp.Data.Active)

	ok, err := model.TokenHasActivePoolSubscription(7, 30)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestGetTokenPoolSubscriptions_List(t *testing.T) {
	setupTokenPoolSubscriptionCheckoutTestDB(t)

	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.Token{
		Id: 1, UserId: 1, Name: "tok-one", Key: "sk-test-token-one-abcdefghijklmnop",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Pool{
		Id: 10, Name: "Lite", Status: model.PoolStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.TokenPoolSubscription{
		TokenId: 1, PoolId: 10, PeriodStart: now, PeriodEnd: now + 100,
	}).Error)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pool/token_subscriptions?pool_id=10", nil)

	GetTokenPoolSubscriptions(ctx)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				TokenId int  `json:"token_id"`
				Active  bool `json:"active"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, int64(1), resp.Data.Total)
	require.Len(t, resp.Data.Items, 1)
	require.Equal(t, 1, resp.Data.Items[0].TokenId)
}

func TestGetTokenPoolSubscriptions_NameFilters(t *testing.T) {
	setupTokenPoolSubscriptionCheckoutTestDB(t)

	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.Token{
		Id: 1, UserId: 1, Name: "tok-one", Key: "sk-test-token-one-abcdefghijklmnop",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Pool{
		Id: 10, Name: "Lite", Status: model.PoolStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.TokenPoolSubscription{
		TokenId: 1, PoolId: 10, PeriodStart: now, PeriodEnd: now + 100,
	}).Error)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pool/token_subscriptions?token_name=tok-one&pool_name=Lite", nil)

	GetTokenPoolSubscriptions(ctx)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				TokenId   int    `json:"token_id"`
				TokenName string `json:"token_name"`
				PoolName  string `json:"pool_name"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, int64(1), resp.Data.Total)
	require.Len(t, resp.Data.Items, 1)
	require.Equal(t, "tok-one", resp.Data.Items[0].TokenName)
	require.Equal(t, "Lite", resp.Data.Items[0].PoolName)
}

func TestGetTokenPoolSubscriptions_VisibilityFilter(t *testing.T) {
	setupTokenPoolSubscriptionCheckoutTestDB(t)

	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.Token{
		Id: 1, UserId: 1, Name: "tok-one", Key: "sk-test-token-one-abcdefghijklmnop",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id: 2, UserId: 1, Name: "tok-two", Key: "sk-test-token-two-abcdefghijklmnop",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id: 3, UserId: 1, Name: "tok-three", Key: "sk-test-token-three-abcdefghijklmnop",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Pool{
		Id: 10, Name: "Lite", Status: model.PoolStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.TokenPoolSubscription{
		TokenId: 1, PoolId: 10, PeriodStart: now, PeriodEnd: now + 3600,
	}).Error)
	require.NoError(t, model.DB.Create(&model.TokenPoolSubscription{
		TokenId: 2, PoolId: 10, PeriodStart: now - 7200, PeriodEnd: now - 10,
	}).Error)
	require.NoError(t, model.DB.Create(&model.TokenPoolSubscription{
		TokenId: 3, PoolId: 10, PeriodStart: now - 7200, PeriodEnd: now - 10, Archived: true,
	}).Error)

	type listResp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				TokenId  int  `json:"token_id"`
				Active   bool `json:"active"`
				Archived bool `json:"archived"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pool/token_subscriptions?pool_id=10&visibility=active", nil)
	GetTokenPoolSubscriptions(ctx)
	require.Equal(t, http.StatusOK, w.Code)
	var resp listResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, int64(1), resp.Data.Total)
	require.Equal(t, 1, resp.Data.Items[0].TokenId)
	require.True(t, resp.Data.Items[0].Active)

	w = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pool/token_subscriptions?pool_id=10&visibility=disabled", nil)
	GetTokenPoolSubscriptions(ctx)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, int64(1), resp.Data.Total)
	require.Equal(t, 2, resp.Data.Items[0].TokenId)

	w = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pool/token_subscriptions?pool_id=10&visibility=archived", nil)
	GetTokenPoolSubscriptions(ctx)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, int64(1), resp.Data.Total)
	require.Equal(t, 3, resp.Data.Items[0].TokenId)
	require.True(t, resp.Data.Items[0].Archived)
}

func TestPutTokenPoolSubscriptionArchive(t *testing.T) {
	setupTokenPoolSubscriptionCheckoutTestDB(t)

	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.Token{
		Id: 11, UserId: 1, Name: "tok11", Key: "sk-test-token-11-abcdefghijklmnopqrstu",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Pool{
		Id: 60, Name: "pool-60", Status: model.PoolStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.TokenPoolSubscription{
		TokenId: 11, PoolId: 60, PeriodStart: now, PeriodEnd: now + 3600,
	}).Error)

	body, _ := json.Marshal(map[string]interface{}{
		"token_id": 11,
		"pool_id":  60,
		"archived": true,
	})
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/pool/token_subscription/archive", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	PutTokenPoolSubscriptionArchive(ctx)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Archived bool `json:"archived"`
			Active   bool `json:"active"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.True(t, resp.Data.Archived)
	require.True(t, resp.Data.Active)
}

func TestPutTokenPoolSubscription_Remark(t *testing.T) {
	setupTokenPoolSubscriptionCheckoutTestDB(t)

	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.Token{
		Id: 9, UserId: 1, Name: "tok9", Key: "sk-test-token-9-abcdefghijklmnopqrstuv",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Pool{
		Id: 50, Name: "pool-50", Status: model.PoolStatusEnabled,
	}).Error)

	future := now + 7*24*3600
	body, _ := json.Marshal(map[string]interface{}{
		"token_id":   9,
		"pool_id":    50,
		"period_end": future,
		"remark":     "ops memo",
	})
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/pool/token_subscription", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	PutTokenPoolSubscription(ctx)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Remark string `json:"remark"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, "ops memo", resp.Data.Remark)

	later := future + 3600
	body, _ = json.Marshal(map[string]interface{}{
		"token_id":   9,
		"pool_id":    50,
		"period_end": later,
	})
	w = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/pool/token_subscription", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	PutTokenPoolSubscription(ctx)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, "ops memo", resp.Data.Remark)
}
