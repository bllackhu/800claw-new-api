package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/wechatpay"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"gorm.io/gorm"
)

func setupTokenPoolSubscriptionCheckoutTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.Token{},
		&model.Pool{},
		&model.PoolBinding{},
		&model.TokenPoolSubscriptionOrder{},
		&model.TokenPoolSubscription{},
	))
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func stubWechatpayClient(t *testing.T) {
	t.Helper()
	wechatpayClientFunc = func(ctx context.Context) (*core.Client, *wechatpay.Config, error) {
		return &core.Client{}, &wechatpay.Config{MchID: "1900000001"}, nil
	}
	t.Cleanup(func() {
		wechatpayClientFunc = wechatpay.Client
	})
}

func TestRequestTokenPoolSubscriptionWechatCheckout_PoolIdMustMatchResolved(t *testing.T) {
	db := setupTokenPoolSubscriptionCheckoutTestDB(t)
	token := seedToken(t, db, 42, "co-tok", "checkout-key-12345678")
	resolved := &model.Pool{Name: "resolved-p", Status: model.PoolStatusEnabled, MonthlyPriceCny: 40}
	require.NoError(t, db.Create(resolved).Error)
	other := &model.Pool{Name: "other-p", Status: model.PoolStatusEnabled, MonthlyPriceCny: 40}
	require.NoError(t, db.Create(other).Error)
	require.NoError(t, db.Create(&model.PoolBinding{
		BindingType:  model.PoolBindingTypeToken,
		BindingValue: strconv.Itoa(token.Id),
		PoolId:       resolved.Id,
		Enabled:      true,
	}).Error)

	body, _ := json.Marshal(map[string]int{"pool_id": other.Id})
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/usage/token/pool/subscription/wechat/checkout", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("token_id", token.Id)
	ctx.Set("id", token.UserId)

	RequestTokenPoolSubscriptionWechatCheckout(ctx)
	require.Equal(t, 200, rec.Code)
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.False(t, resp.Success)
	require.Contains(t, resp.Message, "resolved pool")
}

func TestRequestTokenPoolSubscriptionWechatCheckout_WeChatNotConfigured(t *testing.T) {
	db := setupTokenPoolSubscriptionCheckoutTestDB(t)
	token := seedToken(t, db, 43, "co-tok2", "checkout-key-abcdefgh")
	resolved := &model.Pool{Name: "priced-p", Status: model.PoolStatusEnabled, MonthlyPriceCny: 12.34}
	require.NoError(t, db.Create(resolved).Error)
	require.NoError(t, db.Create(&model.PoolBinding{
		BindingType:  model.PoolBindingTypeToken,
		BindingValue: strconv.Itoa(token.Id),
		PoolId:       resolved.Id,
		Enabled:      true,
	}).Error)

	body, _ := json.Marshal(map[string]int{"pool_id": resolved.Id})
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/usage/token/pool/subscription/wechat/checkout", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("token_id", token.Id)
	ctx.Set("id", token.UserId)

	RequestTokenPoolSubscriptionWechatCheckout(ctx)
	require.Equal(t, 200, rec.Code)
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.False(t, resp.Success)
	require.Contains(t, resp.Message, "wechat pay is not configured")
}

func TestRequestTokenPoolSubscriptionWechatCheckout_ReusesPendingOrder(t *testing.T) {
	db := setupTokenPoolSubscriptionCheckoutTestDB(t)
	token := seedToken(t, db, 44, "reuse-tok", "reuse-checkout-key-1234")
	resolved := &model.Pool{Name: "reuse-pool", Status: model.PoolStatusEnabled, MonthlyPriceCny: 40}
	require.NoError(t, db.Create(resolved).Error)
	require.NoError(t, db.Create(&model.PoolBinding{
		BindingType:  model.PoolBindingTypeToken,
		BindingValue: strconv.Itoa(token.Id),
		PoolId:       resolved.Id,
		Enabled:      true,
	}).Error)

	existingTrade := "TPREUSE123456"
	require.NoError(t, db.Create(&model.TokenPoolSubscriptionOrder{
		UserId:               token.UserId,
		TokenId:              token.Id,
		PoolId:               resolved.Id,
		AmountCny:            40,
		AmountTotalFen:       4000,
		Currency:             "CNY",
		BillingPeriodSeconds: 30 * 24 * 3600,
		TradeNo:              existingTrade,
		CodeUrl:              "weixin://wxpay/bizpayurl?pr=reuse",
		Status:               common.TopUpStatusPending,
		CreateTime:           common.GetTimestamp(),
	}).Error)

	prepayCalls := 0
	nativePrepayFunc = func(ctx context.Context, cfg *wechatpay.Config, client *core.Client, notifyURL, tradeNo, description string, amountFen int64) (string, error) {
		prepayCalls++
		return "", nil
	}
	t.Cleanup(func() {
		nativePrepayFunc = wechatpay.NativePrepay
	})

	body, _ := json.Marshal(map[string]int{"pool_id": resolved.Id})
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/usage/token/pool/subscription/wechat/checkout", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("token_id", token.Id)
	ctx.Set("id", token.UserId)

	RequestTokenPoolSubscriptionWechatCheckout(ctx)
	require.Equal(t, 0, prepayCalls)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			TradeNo  string `json:"trade_no"`
			CodeUrl  string `json:"code_url"`
			Reused   bool   `json:"reused"`
			Status   string `json:"status"`
			PoolName string `json:"pool_name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.True(t, resp.Data.Reused)
	require.Equal(t, existingTrade, resp.Data.TradeNo)
	require.Equal(t, "weixin://wxpay/bizpayurl?pr=reuse", resp.Data.CodeUrl)
	require.Equal(t, common.TopUpStatusPending, resp.Data.Status)
	require.Equal(t, "reuse-pool", resp.Data.PoolName)
}

func TestRequestTokenPoolSubscriptionWechatCheckout_NewOrderWhenAmountMismatch(t *testing.T) {
	db := setupTokenPoolSubscriptionCheckoutTestDB(t)
	stubWechatpayClient(t)
	token := seedToken(t, db, 45, "new-tok", "new-checkout-key-12345")
	resolved := &model.Pool{Name: "priced-pool", Status: model.PoolStatusEnabled, MonthlyPriceCny: 50}
	require.NoError(t, db.Create(resolved).Error)
	require.NoError(t, db.Create(&model.PoolBinding{
		BindingType:  model.PoolBindingTypeToken,
		BindingValue: strconv.Itoa(token.Id),
		PoolId:       resolved.Id,
		Enabled:      true,
	}).Error)

	require.NoError(t, db.Create(&model.TokenPoolSubscriptionOrder{
		UserId:         token.UserId,
		TokenId:        token.Id,
		PoolId:         resolved.Id,
		AmountTotalFen: 4000,
		TradeNo:        "TPOLDAMT",
		CodeUrl:        "weixin://wxpay/bizpayurl?pr=old",
		Status:         common.TopUpStatusPending,
		CreateTime:     common.GetTimestamp(),
	}).Error)

	var newTradeNo string
	nativePrepayFunc = func(ctx context.Context, cfg *wechatpay.Config, client *core.Client, notifyURL, tradeNo, description string, amountFen int64) (string, error) {
		newTradeNo = tradeNo
		require.Equal(t, int64(5000), amountFen)
		return "weixin://wxpay/bizpayurl?pr=new", nil
	}
	t.Cleanup(func() {
		nativePrepayFunc = wechatpay.NativePrepay
	})

	body, _ := json.Marshal(map[string]int{"pool_id": resolved.Id})
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/usage/token/pool/subscription/wechat/checkout", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("token_id", token.Id)
	ctx.Set("id", token.UserId)

	RequestTokenPoolSubscriptionWechatCheckout(ctx)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			TradeNo string `json:"trade_no"`
			Reused  bool   `json:"reused"`
			CodeUrl string `json:"code_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.False(t, resp.Data.Reused)
	require.NotEmpty(t, newTradeNo)
	require.Equal(t, newTradeNo, resp.Data.TradeNo)
	require.Equal(t, "weixin://wxpay/bizpayurl?pr=new", resp.Data.CodeUrl)

	var oldOrder model.TokenPoolSubscriptionOrder
	require.NoError(t, db.Where("trade_no = ?", "TPOLDAMT").First(&oldOrder).Error)
	require.Equal(t, common.TopUpStatusExpired, oldOrder.Status)
}

func TestGetTokenPoolSubscriptionOrderSelf_WrongToken(t *testing.T) {
	db := setupTokenPoolSubscriptionCheckoutTestDB(t)
	owner := seedToken(t, db, 46, "owner", "owner-token-key-123456")
	other := seedToken(t, db, 47, "other", "other-token-key-123456")
	resolved := &model.Pool{Name: "p", Status: model.PoolStatusEnabled, MonthlyPriceCny: 10}
	require.NoError(t, db.Create(resolved).Error)
	require.NoError(t, db.Create(&model.TokenPoolSubscriptionOrder{
		UserId:         owner.UserId,
		TokenId:        owner.Id,
		PoolId:         resolved.Id,
		AmountTotalFen: 1000,
		TradeNo:        "TPOWN123",
		Status:         common.TopUpStatusPending,
		CreateTime:     common.GetTimestamp(),
	}).Error)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/usage/token/pool/subscription/order?trade_no=TPOWN123", nil)
	ctx.Set("token_id", other.Id)

	GetTokenPoolSubscriptionOrderSelf(ctx)
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.False(t, resp.Success)
	require.Contains(t, resp.Message, "not found")
}

func TestGetTokenPoolSubscriptionOrderSelf_ReconcileSuccess(t *testing.T) {
	db := setupTokenPoolSubscriptionCheckoutTestDB(t)
	stubWechatpayClient(t)
	token := seedToken(t, db, 48, "pay-tok", "pay-token-key-12345678")
	resolved := &model.Pool{Name: "pay-pool", Status: model.PoolStatusEnabled, MonthlyPriceCny: 20}
	require.NoError(t, db.Create(resolved).Error)
	tradeNo := "TPPAYTEST01"
	require.NoError(t, db.Create(&model.TokenPoolSubscriptionOrder{
		UserId:               token.UserId,
		TokenId:              token.Id,
		PoolId:               resolved.Id,
		AmountTotalFen:       2000,
		Currency:             "CNY",
		BillingPeriodSeconds: 3600,
		TradeNo:              tradeNo,
		Status:               common.TopUpStatusPending,
		CreateTime:           common.GetTimestamp(),
	}).Error)

	state := "SUCCESS"
	wxTxn := "4200001234"
	total := int64(2000)
	cur := "CNY"
	queryTransactionByOutTradeNoFunc = func(ctx context.Context, cfg *wechatpay.Config, client *core.Client, outTradeNo string) (*payments.Transaction, error) {
		require.Equal(t, tradeNo, outTradeNo)
		return &payments.Transaction{
			OutTradeNo:    &tradeNo,
			TransactionId: &wxTxn,
			TradeState:    &state,
			Amount: &payments.TransactionAmount{
				Total:    &total,
				Currency: &cur,
			},
		}, nil
	}
	t.Cleanup(func() {
		queryTransactionByOutTradeNoFunc = wechatpay.QueryTransactionByOutTradeNo
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/usage/token/pool/subscription/order?trade_no="+tradeNo, nil)
	ctx.Set("token_id", token.Id)

	GetTokenPoolSubscriptionOrderSelf(ctx)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Status               string `json:"status"`
			ReconciledFromWechat bool   `json:"reconciled_from_wechat"`
			PoolName             string `json:"pool_name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, common.TopUpStatusSuccess, resp.Data.Status)
	require.True(t, resp.Data.ReconciledFromWechat)
	require.Equal(t, "pay-pool", resp.Data.PoolName)

	active, err := model.TokenHasActivePoolSubscription(token.Id, resolved.Id)
	require.NoError(t, err)
	require.True(t, active)
}
