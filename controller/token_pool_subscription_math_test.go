package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestComputeAmountFen_FullPriceNoDiscount(t *testing.T) {
	// 40 CNY/mo * 1 month * 10000/10000 = 4000 fen.
	require.Equal(t, int64(4000), computeAmountFen(40, 1, 10000))
	// 40 CNY/mo * 3 months * 10000/10000 = 12000 fen.
	require.Equal(t, int64(12000), computeAmountFen(40, 3, 10000))
}

func TestComputeAmountFen_TieredDiscounts(t *testing.T) {
	// Plan document: 1m/10000, 3m/9600, 6m/9000, 12m/8000.
	// Monthly price 40 CNY -> 4000 fen/month.
	tests := []struct {
		name    string
		monthly float64
		months  int
		bp      int
		want    int64
	}{
		{"3m 96%", 40, 3, 9600, 11520},   // 4000 * 3 * 0.96 = 11520
		{"6m 90%", 40, 6, 9000, 21600},   // 4000 * 6 * 0.90 = 21600
		{"12m 80%", 40, 12, 8000, 38400}, // 4000 * 12 * 0.80 = 38400
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, computeAmountFen(tt.monthly, tt.months, tt.bp))
		})
	}
}

func TestComputeAmountFen_FractionalMonthlyRounding(t *testing.T) {
	// 12.34 CNY/mo -> 1234 fen. 1 * 100% = 1234.
	require.Equal(t, int64(1234), computeAmountFen(12.34, 1, 10000))
	// 12.34 * 3 months * 0.96 = 3553.92 -> round to 3554.
	require.Equal(t, int64(3554), computeAmountFen(12.34, 3, 9600))
}

func TestComputeAmountFen_DefaultsAndGuards(t *testing.T) {
	// discount_ratio_bp <= 0 defaults to 10000 (full price).
	require.Equal(t, int64(4000), computeAmountFen(40, 1, 0))
	require.Equal(t, int64(4000), computeAmountFen(40, 1, -1))
	// non-positive months / price yield 0 (no charge).
	require.Equal(t, int64(0), computeAmountFen(40, 0, 10000))
	require.Equal(t, int64(0), computeAmountFen(0, 3, 9600))
}

func TestComputeUpgradeCreditSeconds_ValueConversion(t *testing.T) {
	// 10 days remaining on 40 CNY/mo => 10 * 40 / 100 = 4 days on 100 CNY/mo.
	tenDays := int64(10 * 86400)
	require.Equal(t, int64(4*86400), computeUpgradeCreditSeconds(tenDays, 40, 100))

	// Same tier round-trip: credit == remaining.
	require.Equal(t, tenDays, computeUpgradeCreditSeconds(tenDays, 40, 40))
}

func TestComputeUpgradeCreditSeconds_FloorsFractional(t *testing.T) {
	// 100 seconds at 1:3 value ratio -> 33.33... -> floored to 33.
	require.Equal(t, int64(33), computeUpgradeCreditSeconds(100, 1, 3))
	// Ensure result is strictly floored: 1 remaining second at 1:3 ratio -> 0.
	require.Equal(t, int64(0), computeUpgradeCreditSeconds(1, 1, 3))
	// Clean 1:2 conversion — no rounding drift.
	require.Equal(t, int64(15*86400), computeUpgradeCreditSeconds(int64(30*86400), 50, 100))
}

func TestComputeUpgradeCreditSeconds_GuardsAgainstZero(t *testing.T) {
	require.Equal(t, int64(0), computeUpgradeCreditSeconds(0, 40, 100))
	require.Equal(t, int64(0), computeUpgradeCreditSeconds(-1, 40, 100))
	require.Equal(t, int64(0), computeUpgradeCreditSeconds(10*86400, 0, 100))
	require.Equal(t, int64(0), computeUpgradeCreditSeconds(10*86400, 40, 0))
}

func TestFindPeriodOption_MatchesEnabledOnly(t *testing.T) {
	options := []model.PoolPeriodOption{
		{PeriodMonths: 1, DiscountRatioBp: 10000, Enabled: true},
		{PeriodMonths: 3, DiscountRatioBp: 9600, Enabled: false},
		{PeriodMonths: 6, DiscountRatioBp: 9000, Enabled: true},
	}
	opt, ok := findPeriodOption(options, 1)
	require.True(t, ok)
	require.Equal(t, 1, opt.PeriodMonths)

	opt, ok = findPeriodOption(options, 6)
	require.True(t, ok)
	require.Equal(t, 9000, opt.DiscountRatioBp)

	_, ok = findPeriodOption(options, 3)
	require.False(t, ok, "disabled option must not match")

	_, ok = findPeriodOption(options, 12)
	require.False(t, ok, "missing option must not match")
}

func TestBuildPeriodOptionDTOs_ProducesBaseAndDiscountedFen(t *testing.T) {
	options := []model.PoolPeriodOption{
		{PeriodMonths: 1, DiscountRatioBp: 10000, Enabled: true},
		{PeriodMonths: 3, DiscountRatioBp: 9600, Enabled: true},
		{PeriodMonths: 12, DiscountRatioBp: 8000, Enabled: true},
		{PeriodMonths: 24, DiscountRatioBp: 7000, Enabled: false},
	}
	dtos := buildPeriodOptionDTOs(40, options)
	require.Len(t, dtos, 3, "disabled options are excluded")

	// 1 month at full price.
	require.Equal(t, 1, dtos[0].PeriodMonths)
	require.Equal(t, int64(4000), dtos[0].BaseAmountFen)
	require.Equal(t, int64(4000), dtos[0].AmountFen)

	// 3 months: base 12000, discounted 11520 (base_amount_fen always reflects no discount).
	require.Equal(t, int64(12000), dtos[1].BaseAmountFen)
	require.Equal(t, int64(11520), dtos[1].AmountFen)

	// 12 months: base 48000, discounted 38400.
	require.Equal(t, int64(48000), dtos[2].BaseAmountFen)
	require.Equal(t, int64(38400), dtos[2].AmountFen)
}

func TestDefaultPoolPeriodOptions_MatchesPlanDocument(t *testing.T) {
	defaults := defaultPoolPeriodOptions()
	require.Len(t, defaults, 4)

	expected := []struct {
		months int
		bp     int
	}{
		{1, 10000},
		{3, 9600},
		{6, 9000},
		{12, 8000},
	}
	for i, want := range expected {
		require.Equal(t, want.months, defaults[i].PeriodMonths)
		require.Equal(t, want.bp, defaults[i].DiscountRatioBp)
		require.True(t, defaults[i].Enabled)
	}
}
