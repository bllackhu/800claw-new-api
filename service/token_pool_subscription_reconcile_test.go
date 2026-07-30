package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/wechatpay"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"gorm.io/gorm"
)

func setupPoolSubReconcileTestDB(t *testing.T) {
	t.Helper()
	common.UsingSQLite = true
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.TokenPoolSubscriptionOrder{}, &model.TokenPoolSubscription{}))
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestMaybeReconcilePendingPoolSubscription_NoPendingSkipsWeChat(t *testing.T) {
	setupPoolSubReconcileTestDB(t)
	queried := false
	restore := SetTokenPoolSubscriptionReconcileTestHooks(
		func(ctx context.Context) (*core.Client, *wechatpay.Config, error) {
			return &core.Client{}, &wechatpay.Config{MchID: "m"}, nil
		},
		func(ctx context.Context, cfg *wechatpay.Config, client *core.Client, outTradeNo string) (*payments.Transaction, error) {
			queried = true
			return nil, nil
		},
	)
	t.Cleanup(restore)

	reconciled, err := MaybeReconcilePendingPoolSubscription(context.Background(), 1, 10)
	require.NoError(t, err)
	require.False(t, reconciled)
	require.False(t, queried)
}

func TestMaybeReconcilePendingPoolSubscription_FulfillsOnSuccess(t *testing.T) {
	setupPoolSubReconcileTestDB(t)
	tradeNo := "TPRECON1"
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.TokenPoolSubscriptionOrder{
		TokenId:              5,
		PoolId:               20,
		AmountTotalFen:       1000,
		Currency:             "CNY",
		BillingPeriodSeconds: 86400,
		TradeNo:              tradeNo,
		Status:               common.TopUpStatusPending,
		CreateTime:           now,
	}).Error)

	state := "SUCCESS"
	total := int64(1000)
	cur := "CNY"
	restore := SetTokenPoolSubscriptionReconcileTestHooks(
		func(ctx context.Context) (*core.Client, *wechatpay.Config, error) {
			return &core.Client{}, &wechatpay.Config{MchID: "m"}, nil
		},
		func(ctx context.Context, cfg *wechatpay.Config, client *core.Client, outTradeNo string) (*payments.Transaction, error) {
			require.Equal(t, tradeNo, outTradeNo)
			return &payments.Transaction{
				OutTradeNo: &tradeNo,
				TradeState: &state,
				Amount: &payments.TransactionAmount{
					Total:    &total,
					Currency: &cur,
				},
			}, nil
		},
	)
	t.Cleanup(restore)

	reconciled, err := MaybeReconcilePendingPoolSubscription(context.Background(), 5, 20)
	require.NoError(t, err)
	require.True(t, reconciled)

	ok, err := model.TokenHasActivePoolSubscription(5, 20)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestReconcileTokenPoolSubscriptionOrderFromWeChat_ExpiredOrder(t *testing.T) {
	setupPoolSubReconcileTestDB(t)
	tradeNo := "TPEXPIREDREC1"
	now := common.GetTimestamp()
	order := &model.TokenPoolSubscriptionOrder{
		TokenId:              7,
		PoolId:               30,
		AmountTotalFen:       2000,
		Currency:             "CNY",
		BillingPeriodSeconds: 86400,
		TradeNo:              tradeNo,
		Status:               common.TopUpStatusExpired,
		CreateTime:           now,
	}
	require.NoError(t, model.DB.Create(order).Error)

	state := "SUCCESS"
	total := int64(2000)
	cur := "CNY"
	restore := SetTokenPoolSubscriptionReconcileTestHooks(
		func(ctx context.Context) (*core.Client, *wechatpay.Config, error) {
			return &core.Client{}, &wechatpay.Config{MchID: "m"}, nil
		},
		func(ctx context.Context, cfg *wechatpay.Config, client *core.Client, outTradeNo string) (*payments.Transaction, error) {
			return &payments.Transaction{
				OutTradeNo: &tradeNo,
				TradeState: &state,
				Amount: &payments.TransactionAmount{
					Total:    &total,
					Currency: &cur,
				},
			}, nil
		},
	)
	t.Cleanup(restore)

	reconciled, err := ReconcileTokenPoolSubscriptionOrderFromWeChat(context.Background(), order)
	require.NoError(t, err)
	require.True(t, reconciled)

	ok, err := model.TokenHasActivePoolSubscription(7, 30)
	require.NoError(t, err)
	require.True(t, ok)
}
