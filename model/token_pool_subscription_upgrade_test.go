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

// setupUpgradeTestDB installs a per-test in-memory SQLite DB and rebinds model.DB.
// Mirrors setupTokenPoolSubscriptionAdminTestDB but scoped to files that need
// PoolPeriodOption in their schema.
func setupUpgradeTestDB(t *testing.T) {
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
		&PoolPeriodOption{},
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

// TestUpgradeTokenPoolSubscriptionTx_ClosesOldAndOpensNew verifies that upgrading
// atomically shortens the outgoing Lite subscription (period_end = now-1) and
// creates the Pro subscription with paid period + credit seconds folded in.
func TestUpgradeTokenPoolSubscriptionTx_ClosesOldAndOpensNew(t *testing.T) {
	setupUpgradeTestDB(t)

	const tokenId = 700
	const litePoolId = 10
	const proPoolId = 20
	now := common.GetTimestamp()

	liteEnd := now + 20*86400
	require.NoError(t, DB.Create(&TokenPoolSubscription{
		TokenId:     tokenId,
		PoolId:      litePoolId,
		PeriodStart: now - 10*86400,
		PeriodEnd:   liteEnd,
		LastOrderId: 500,
	}).Error)

	proPeriod := int64(30 * 86400)
	credit := int64(4 * 86400)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return UpgradeTokenPoolSubscriptionTx(tx, tokenId, litePoolId, proPoolId, 501, proPeriod, credit, now)
	}))

	oldSub, err := GetTokenPoolSubscription(tokenId, litePoolId)
	require.NoError(t, err)
	require.NotNil(t, oldSub)
	require.Equal(t, now-1, oldSub.PeriodEnd, "old lite window must be closed at now-1")

	proSub, err := GetTokenPoolSubscription(tokenId, proPoolId)
	require.NoError(t, err)
	require.NotNil(t, proSub)
	require.Equal(t, 501, proSub.LastOrderId)
	expected := periodEndAtBillingEOD(now, proPeriod+credit)
	require.Equal(t, expected, proSub.PeriodEnd)
}

// TestUpgradeTokenPoolSubscriptionTx_NoopWhenOldExpired verifies that when the
// caller has no active Lite window, the upgrade still opens the new Pro row
// but does NOT rewind an already-expired old row backwards.
func TestUpgradeTokenPoolSubscriptionTx_NoopWhenOldExpired(t *testing.T) {
	setupUpgradeTestDB(t)

	const tokenId = 701
	const litePoolId = 10
	const proPoolId = 20
	now := common.GetTimestamp()

	expiredEnd := now - 100
	require.NoError(t, DB.Create(&TokenPoolSubscription{
		TokenId:     tokenId,
		PoolId:      litePoolId,
		PeriodStart: now - 40*86400,
		PeriodEnd:   expiredEnd,
		LastOrderId: 600,
	}).Error)

	proPeriod := int64(30 * 86400)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return UpgradeTokenPoolSubscriptionTx(tx, tokenId, litePoolId, proPoolId, 601, proPeriod, 0, now)
	}))

	oldSub, err := GetTokenPoolSubscription(tokenId, litePoolId)
	require.NoError(t, err)
	require.Equal(t, expiredEnd, oldSub.PeriodEnd, "already-expired lite row must not be rewritten")

	proSub, err := GetTokenPoolSubscription(tokenId, proPoolId)
	require.NoError(t, err)
	require.NotNil(t, proSub)
	require.Equal(t, periodEndAtBillingEOD(now, proPeriod), proSub.PeriodEnd)
}

func TestUpgradeTokenPoolSubscriptionTx_RejectsInvalidArgs(t *testing.T) {
	setupUpgradeTestDB(t)

	err := DB.Transaction(func(tx *gorm.DB) error {
		return UpgradeTokenPoolSubscriptionTx(tx, 1, 10, 10, 1, 30*86400, 0, common.GetTimestamp())
	})
	require.Error(t, err, "same from/to pool must fail")

	err = DB.Transaction(func(tx *gorm.DB) error {
		return UpgradeTokenPoolSubscriptionTx(tx, 0, 10, 20, 1, 30*86400, 0, common.GetTimestamp())
	})
	require.Error(t, err, "zero token_id must fail")
}

// TestCompleteTokenPoolSubscriptionFromNotify_MultiPeriodExtendsByMonths verifies
// that a paid order with PeriodMonths > 1 stacks the full multi-month window
// (BillingPeriodSeconds * PeriodMonths) onto the subscription.
func TestCompleteTokenPoolSubscriptionFromNotify_MultiPeriodExtendsByMonths(t *testing.T) {
	setupUpgradeTestDB(t)

	now := common.GetTimestamp()
	order := &TokenPoolSubscriptionOrder{
		UserId:               1,
		TokenId:              1,
		PoolId:               10,
		AmountCny:            108,
		AmountTotalFen:       10800,
		Currency:             "CNY",
		BillingPeriodSeconds: 30 * 86400,
		PeriodMonths:         3,
		DiscountRatioBp:      9600,
		TradeNo:              "TPMULTI3M01",
		Status:               common.TopUpStatusPending,
		CreateTime:           now,
	}
	require.NoError(t, InsertTokenPoolSubscriptionOrder(order))

	raw := `{"trade_state":"SUCCESS"}`
	require.NoError(t, CompleteTokenPoolSubscriptionFromNotify("TPMULTI3M01", "wx-txn-multi", raw, 10800, "CNY"))

	sub, err := GetTokenPoolSubscription(1, 10)
	require.NoError(t, err)
	require.NotNil(t, sub)
	expected := periodEndAtBillingEOD(now, 3*30*86400)
	require.Equal(t, expected, sub.PeriodEnd)
}

// TestCompleteTokenPoolSubscriptionFromNotify_UpgradeIdempotent verifies that
// replaying the WeChat callback for an upgrade order does not re-close the
// (already-closed) old sub or double-credit the new one.
func TestCompleteTokenPoolSubscriptionFromNotify_UpgradeIdempotent(t *testing.T) {
	setupUpgradeTestDB(t)

	const tokenId = 900
	const litePoolId = 30
	const proPoolId = 40
	now := common.GetTimestamp()

	liteEnd := now + 15*86400
	require.NoError(t, DB.Create(&TokenPoolSubscription{
		TokenId:     tokenId,
		PoolId:      litePoolId,
		PeriodStart: now - 5*86400,
		PeriodEnd:   liteEnd,
		LastOrderId: 700,
	}).Error)

	credit := int64(6 * 86400)
	order := &TokenPoolSubscriptionOrder{
		UserId:               1,
		TokenId:              tokenId,
		PoolId:               proPoolId,
		AmountCny:            100,
		AmountTotalFen:       10000,
		Currency:             "CNY",
		BillingPeriodSeconds: 30 * 86400,
		PeriodMonths:         1,
		DiscountRatioBp:      10000,
		IsUpgrade:            true,
		UpgradedFromPoolId:   litePoolId,
		CreditSecondsGranted: credit,
		TradeNo:              "TPUPGR01",
		Status:               common.TopUpStatusPending,
		CreateTime:           now,
	}
	require.NoError(t, InsertTokenPoolSubscriptionOrder(order))

	raw := `{"trade_state":"SUCCESS"}`
	require.NoError(t, CompleteTokenPoolSubscriptionFromNotify("TPUPGR01", "wx-txn-upgr", raw, 10000, "CNY"))
	require.NoError(t, CompleteTokenPoolSubscriptionFromNotify("TPUPGR01", "wx-txn-upgr", raw, 10000, "CNY"))

	liteSub, err := GetTokenPoolSubscription(tokenId, litePoolId)
	require.NoError(t, err)
	require.Equal(t, now-1, liteSub.PeriodEnd)

	proSub, err := GetTokenPoolSubscription(tokenId, proPoolId)
	require.NoError(t, err)
	require.NotNil(t, proSub)
	expected := periodEndAtBillingEOD(now, 30*86400+credit)
	require.Equal(t, expected, proSub.PeriodEnd)

	var loaded TokenPoolSubscriptionOrder
	require.NoError(t, DB.Where("trade_no = ?", "TPUPGR01").First(&loaded).Error)
	require.Equal(t, common.TopUpStatusSuccess, loaded.Status)
}
