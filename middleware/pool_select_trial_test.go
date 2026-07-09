package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var (
	poolSelectTestDBOnce sync.Once
	poolSelectTestDB     *gorm.DB
)

// setupPoolSelectTestDB installs a shared in-memory SQLite DB with the tables
// PoolSelect touches (pools, pool_bindings, tokens, token_pool_subscriptions).
// It's cheap enough to run in the middleware test package: no channel/log/user rows.
func setupPoolSelectTestDB(t *testing.T) {
	t.Helper()
	poolSelectTestDBOnce.Do(func() {
		db, err := gorm.Open(sqlite.Open("file:pool_select_test?mode=memory&cache=shared"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(
			&model.Pool{},
			&model.PoolBinding{},
			&model.TokenPoolSubscription{},
			&model.TokenPoolSubscriptionOrder{},
		))
		poolSelectTestDB = db
	})
	model.DB = poolSelectTestDB
	model.LOG_DB = poolSelectTestDB
	common.UsingSQLite = true
	common.PoolEnabled = true

	// Every test starts from a clean slate. Keeping AutoMigrate outside the
	// once-block avoids paying migration cost per test.
	poolSelectTestDB.Exec("DELETE FROM token_pool_subscriptions")
	poolSelectTestDB.Exec("DELETE FROM token_pool_subscription_orders")
	poolSelectTestDB.Exec("DELETE FROM pool_bindings")
	poolSelectTestDB.Exec("DELETE FROM pools")

	t.Cleanup(func() {
		common.PoolEnabled = false
	})
}

func seedPricedPoolWithTokenBinding(t *testing.T, tokenId int) *model.Pool {
	t.Helper()
	pool := &model.Pool{
		Name:                 "priced_pool_" + t.Name(),
		Status:               model.PoolStatusEnabled,
		MonthlyPriceCny:      40,
		BillingCurrency:      "CNY",
		BillingPeriodSeconds: 30 * 86400,
	}
	require.NoError(t, model.DB.Create(pool).Error)
	require.NoError(t, model.DB.Create(&model.PoolBinding{
		BindingType:  model.PoolBindingTypeToken,
		BindingValue: strconv.Itoa(tokenId),
		PoolId:       pool.Id,
		Enabled:      true,
	}).Error)
	return pool
}

func newPoolSelectTestRequest(t *testing.T, tokenId int, requireSub bool) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req, err := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	require.NoError(t, err)
	c.Request = req
	common.SetContextKey(c, constant.ContextKeyUserId, 1)
	common.SetContextKey(c, constant.ContextKeyTokenId, tokenId)
	common.SetContextKey(c, constant.ContextKeyTokenRequirePoolSubscription, requireSub)
	return rec, c
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestPoolSelect_FirstRequestGrantsFreeMonth(t *testing.T) {
	setupPoolSelectTestDB(t)
	tokenId := 501
	pool := seedPricedPoolWithTokenBinding(t, tokenId)

	rec, c := newPoolSelectTestRequest(t, tokenId, true)
	called := false
	handler := PoolSelect()
	handler(c)
	if !c.IsAborted() {
		called = true
		c.Next()
	}
	require.True(t, called, "middleware must not abort on the first request; it should grant a trial and continue")
	require.NotEqual(t, http.StatusPaymentRequired, rec.Code)

	sub, err := model.GetTokenPoolSubscription(tokenId, pool.Id)
	require.NoError(t, err)
	require.NotNil(t, sub, "a trial row must exist after first request")
	require.Greater(t, sub.PeriodEnd, common.GetTimestamp())
	require.Equal(t, 0, sub.LastOrderId)
}

func TestPoolSelect_SecondCallAfterExpiryReturns402(t *testing.T) {
	setupPoolSelectTestDB(t)
	tokenId := 502
	pool := seedPricedPoolWithTokenBinding(t, tokenId)

	// Seed an already-expired trial to simulate "customer consumed their free window".
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.TokenPoolSubscription{
		TokenId:     tokenId,
		PoolId:      pool.Id,
		PeriodStart: now - 40*86400,
		PeriodEnd:   now - 3600,
		LastOrderId: 0,
	}).Error)

	rec, c := newPoolSelectTestRequest(t, tokenId, true)
	handler := PoolSelect()
	handler(c)
	require.True(t, c.IsAborted(), "expired trial must not re-grant; the request must be aborted")
	require.Equal(t, http.StatusPaymentRequired, rec.Code)

	// And confirm the expired row was not silently extended.
	sub, err := model.GetTokenPoolSubscription(tokenId, pool.Id)
	require.NoError(t, err)
	require.Equal(t, now-3600, sub.PeriodEnd)
}

func TestPoolSelect_NoRequireSubBypassesGate(t *testing.T) {
	setupPoolSelectTestDB(t)
	tokenId := 503
	pool := seedPricedPoolWithTokenBinding(t, tokenId)

	rec, c := newPoolSelectTestRequest(t, tokenId, false)
	handler := PoolSelect()
	handler(c)
	require.False(t, c.IsAborted(), "tokens without require_pool_subscription must not hit the gate")
	require.NotEqual(t, http.StatusPaymentRequired, rec.Code)

	// No trial row should be auto-created for opted-out tokens.
	var count int64
	require.NoError(t, model.DB.Model(&model.TokenPoolSubscription{}).
		Where("token_id = ? AND pool_id = ?", tokenId, pool.Id).
		Count(&count).Error)
	require.Equal(t, int64(0), count)
}
