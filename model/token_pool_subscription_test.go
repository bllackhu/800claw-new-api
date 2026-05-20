package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestCompleteTokenPoolSubscriptionFromNotify_Idempotent(t *testing.T) {
	truncateTables(t)
	DB.Exec("DELETE FROM token_pool_subscription_orders")
	DB.Exec("DELETE FROM token_pool_subscriptions")

	now := common.GetTimestamp()
	o := &TokenPoolSubscriptionOrder{
		UserId:               1,
		TokenId:              1,
		PoolId:               10,
		AmountCny:            40,
		AmountTotalFen:       4000,
		Currency:             "CNY",
		BillingPeriodSeconds: 3600,
		TradeNo:              "TPTESTNOTIFY1",
		Status:               common.TopUpStatusPending,
		CreateTime:           now,
	}
	require.NoError(t, InsertTokenPoolSubscriptionOrder(o))

	raw := `{"trade_state":"SUCCESS"}`
	require.NoError(t, CompleteTokenPoolSubscriptionFromNotify("TPTESTNOTIFY1", "wx-txn-1", raw, 4000, "CNY"))
	require.NoError(t, CompleteTokenPoolSubscriptionFromNotify("TPTESTNOTIFY1", "wx-txn-1", raw, 4000, "CNY"))

	var loaded TokenPoolSubscriptionOrder
	require.NoError(t, DB.Where("trade_no = ?", "TPTESTNOTIFY1").First(&loaded).Error)
	require.Equal(t, common.TopUpStatusSuccess, loaded.Status)

	ok, err := TokenHasActivePoolSubscription(1, 10)
	require.NoError(t, err)
	require.True(t, ok)

	sub, err := GetTokenPoolSubscription(1, 10)
	require.NoError(t, err)
	require.NotNil(t, sub)
	require.Greater(t, sub.PeriodEnd, now)
}
