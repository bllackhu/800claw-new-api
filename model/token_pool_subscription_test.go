package model

import (
	"fmt"
	"strings"
	"testing"

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
