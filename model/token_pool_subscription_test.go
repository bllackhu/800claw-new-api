package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTokenPoolSubscriptionAdminTestDB(t *testing.T) {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&Token{},
		&Pool{},
		&TokenPoolSubscription{},
		&TokenPoolSubscriptionOrder{},
	))
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

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
		BillingPeriodSeconds: 30 * 86400,
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
	expectedEnd := periodEndAtBillingEOD(now, 30*86400)
	require.Equal(t, expectedEnd, sub.PeriodEnd)
}

func TestPeriodEndAtBillingEOD(t *testing.T) {
	loc := poolSubscriptionLocation
	anchor := time.Date(2026, 6, 10, 14, 37, 22, 0, loc).Unix()

	tests := []struct {
		name          string
		periodSeconds int64
		want          time.Time
	}{
		{
			name:          "30 day pool",
			periodSeconds: 30 * 86400,
			want:          time.Date(2026, 7, 10, 23, 59, 59, 0, loc),
		},
		{
			name:          "1 day pool",
			periodSeconds: 86400,
			want:          time.Date(2026, 6, 11, 23, 59, 59, 0, loc),
		},
		{
			name:          "sub day clamped to 1 day",
			periodSeconds: 3600,
			want:          time.Date(2026, 6, 11, 23, 59, 59, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := periodEndAtBillingEOD(anchor, tt.periodSeconds)
			require.Equal(t, tt.want.Unix(), got)
		})
	}
}

func TestPeriodEndAtBillingEOD_RenewalStacking(t *testing.T) {
	loc := poolSubscriptionLocation
	existingEnd := time.Date(2026, 7, 10, 23, 59, 59, 0, loc).Unix()
	want := time.Date(2026, 8, 9, 23, 59, 59, 0, loc).Unix()

	got := periodEndAtBillingEOD(existingEnd, 30*86400)
	require.Equal(t, want, got)
}

func TestPeriodEndAtBillingEOD_ActiveGatingBoundary(t *testing.T) {
	loc := poolSubscriptionLocation
	anchor := time.Date(2026, 6, 10, 14, 37, 22, 0, loc).Unix()
	periodEnd := periodEndAtBillingEOD(anchor, 86400)

	require.True(t, periodEnd >= periodEnd-9, "active through 23:59:50 on expiry day")
	require.False(t, periodEnd >= periodEnd+1, "inactive after expiry second")
}

func TestUpsertTokenPoolSubscriptionTx_EODNewAndRenewal(t *testing.T) {
	truncateTables(t)
	DB.Exec("DELETE FROM token_pool_subscription_orders")
	DB.Exec("DELETE FROM token_pool_subscriptions")

	loc := poolSubscriptionLocation
	anchor := time.Date(2026, 6, 10, 14, 37, 22, 0, loc).Unix()
	period30 := int64(30 * 86400)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return upsertTokenPoolSubscriptionTx(tx, 1, 10, 100, period30, anchor)
	}))

	sub, err := GetTokenPoolSubscription(1, 10)
	require.NoError(t, err)
	require.Equal(t, periodEndAtBillingEOD(anchor, period30), sub.PeriodEnd)
	require.Equal(t, 100, sub.LastOrderId)

	renewTime := time.Date(2026, 6, 15, 10, 0, 0, 0, loc).Unix()
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return upsertTokenPoolSubscriptionTx(tx, 1, 10, 101, period30, renewTime)
	}))

	sub2, err := GetTokenPoolSubscription(1, 10)
	require.NoError(t, err)
	require.Equal(t, periodEndAtBillingEOD(sub.PeriodEnd, period30), sub2.PeriodEnd)
	require.Equal(t, 101, sub2.LastOrderId)
	require.Greater(t, sub2.PeriodEnd, sub.PeriodEnd)
}

func TestAdminUpsertTokenPoolSubscription_CreateAndUpdate(t *testing.T) {
	setupTokenPoolSubscriptionAdminTestDB(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&Token{Id: 5, UserId: 1, Name: "t5", Key: "sk-test-5"}).Error)
	require.NoError(t, DB.Create(&Pool{Id: 20, Name: "pool-20", Status: PoolStatusEnabled}).Error)

	future := now + 30*24*3600
	sub, err := AdminUpsertTokenPoolSubscription(5, 20, 0, future)
	require.NoError(t, err)
	require.NotNil(t, sub)
	require.Equal(t, 5, sub.TokenId)
	require.Equal(t, 20, sub.PoolId)
	require.Equal(t, int64(0), int64(sub.LastOrderId))
	require.Equal(t, future, sub.PeriodEnd)

	ok, err := TokenHasActivePoolSubscription(5, 20)
	require.NoError(t, err)
	require.True(t, ok)

	past := now - 3600
	sub2, err := AdminUpsertTokenPoolSubscription(5, 20, 0, past)
	require.NoError(t, err)
	require.Equal(t, past, sub2.PeriodEnd)

	ok, err = TokenHasActivePoolSubscription(5, 20)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestListTokenPoolSubscriptions_Filters(t *testing.T) {
	setupTokenPoolSubscriptionAdminTestDB(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TokenPoolSubscription{
		TokenId: 1, PoolId: 10, PeriodStart: now, PeriodEnd: now + 100,
	}).Error)
	require.NoError(t, DB.Create(&TokenPoolSubscription{
		TokenId: 2, PoolId: 10, PeriodStart: now, PeriodEnd: now + 200,
	}).Error)

	items, total, err := ListTokenPoolSubscriptions(0, 10, 1, 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, 1, items[0].TokenId)

	items, total, err = ListTokenPoolSubscriptions(0, 10, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, items, 2)
}
