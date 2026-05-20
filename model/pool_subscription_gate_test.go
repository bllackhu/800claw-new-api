package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPoolMonthlyPriceCnyDecimalRoundTrip(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Pool{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM pools")
	})

	pool := &Pool{
		Name:            "decimal-price-pool",
		Status:          PoolStatusEnabled,
		MonthlyPriceCny: 12.34,
	}
	require.NoError(t, DB.Create(pool).Error)

	loaded := &Pool{}
	require.NoError(t, DB.Where("id = ?", pool.Id).First(loaded).Error)
	require.InDelta(t, 12.34, loaded.MonthlyPriceCny, 0.001)
}

func TestTokenRelayRequiresPoolSubscriptionCheck(t *testing.T) {
	freePool := &Pool{MonthlyPriceCny: 0}
	paidPool := &Pool{MonthlyPriceCny: 10}

	if TokenRelayRequiresPoolSubscriptionCheck(nil, true) {
		t.Fatal("nil pool")
	}
	if TokenRelayRequiresPoolSubscriptionCheck(freePool, true) {
		t.Fatal("free pool + require should be false")
	}
	if TokenRelayRequiresPoolSubscriptionCheck(paidPool, false) {
		t.Fatal("paid pool + no token opt-in should be false")
	}
	if !TokenRelayRequiresPoolSubscriptionCheck(paidPool, true) {
		t.Fatal("paid pool + token opt-in should be true")
	}
}
