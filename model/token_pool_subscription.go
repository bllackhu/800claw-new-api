package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrPoolSubscriptionOrderUnfulfillable marks a notify/fulfill failure that will not succeed on retry
// (e.g. amount or currency mismatch). Callers should ack WeChat with success to stop retries.
var ErrPoolSubscriptionOrderUnfulfillable = errors.New("pool subscription order unfulfillable")

func isTokenPoolSubscriptionOrderFulfillable(status string) bool {
	return status == common.TopUpStatusPending || status == common.TopUpStatusExpired
}

// TokenPoolSubscriptionOrder records a WeChat Pay pool subscription checkout (native or JSAPI).
type TokenPoolSubscriptionOrder struct {
	Id                   int     `json:"id"`
	UserId               int     `json:"user_id" gorm:"index"`
	TokenId              int     `json:"token_id" gorm:"index:idx_tp_sub_order_token_pool,priority:1"`
	PoolId               int     `json:"pool_id" gorm:"index:idx_tp_sub_order_token_pool,priority:2"`
	AmountCny            float64 `json:"amount_cny"`
	AmountTotalFen       int64   `json:"amount_total_fen" gorm:"bigint"`
	Currency             string  `json:"currency" gorm:"type:varchar(8);default:'CNY'"`
	BillingPeriodSeconds int64   `json:"billing_period_seconds" gorm:"bigint"`
	// PeriodMonths is the number of billing months this order pays for (1 = single-month, matches legacy behavior).
	PeriodMonths int `json:"period_months" gorm:"default:1"`
	// DiscountRatioBp is the discount ratio in basis points (10000 = 100% = no discount).
	DiscountRatioBp int `json:"discount_ratio_bp" gorm:"default:10000"`
	// IsUpgrade indicates this order carries an upgrade payload (UpgradedFromPoolId + CreditSecondsGranted are meaningful).
	IsUpgrade bool `json:"is_upgrade" gorm:"default:false;index"`
	// UpgradedFromPoolId is the pool_id being upgraded away from (Lite side), only when IsUpgrade = true.
	UpgradedFromPoolId int `json:"upgraded_from_pool_id" gorm:"default:0;index"`
	// CreditSecondsGranted is the additional paid-window seconds credited from the old plan's remaining time.
	CreditSecondsGranted int64  `json:"credit_seconds_granted" gorm:"bigint;default:0"`
	TradeNo              string `json:"trade_no" gorm:"type:varchar(64);uniqueIndex"`
	CodeUrl              string `json:"-" gorm:"type:text"`
	// PaymentType discriminates the payment method: "native" or "jsapi".
	PaymentType string `json:"payment_type" gorm:"type:varchar(16);default:'native'"`
	// PrepayId is the JSAPI prepay session ID (WeChat Pay JSAPI only).
	PrepayId string `json:"prepay_id" gorm:"type:varchar(64);default:''"`
	// Openid is the payer's openid in the JSAPI appid (JSAPI only).
	Openid             string `json:"openid" gorm:"type:varchar(64);default:''"`
	WechatTransactionId string `json:"wechat_transaction_id" gorm:"type:varchar(64);default:''"`
	Status              string `json:"status" gorm:"type:varchar(32);index"`
	RawNotify           string `json:"raw_notify" gorm:"type:text"`
	CreateTime          int64  `json:"create_time" gorm:"bigint;index"`
	CompleteTime        int64  `json:"complete_time" gorm:"bigint"`
}

func (TokenPoolSubscriptionOrder) TableName() string {
	return "token_pool_subscription_orders"
}

// TokenPoolSubscription is the active paid window for (token_id, pool_id).
type TokenPoolSubscription struct {
	Id          int   `json:"id"`
	TokenId     int   `json:"token_id" gorm:"uniqueIndex:uk_tp_token_pool,priority:1"`
	PoolId      int   `json:"pool_id" gorm:"uniqueIndex:uk_tp_token_pool,priority:2"`
	PeriodStart int64 `json:"period_start" gorm:"bigint;index"`
	PeriodEnd   int64 `json:"period_end" gorm:"bigint;index"`
	LastOrderId int   `json:"last_order_id" gorm:"default:0"`
	UpdatedAt   int64 `json:"updated_at" gorm:"bigint"`
}

func (TokenPoolSubscription) TableName() string {
	return "token_pool_subscriptions"
}

func (s *TokenPoolSubscription) BeforeCreate(tx *gorm.DB) error {
	s.UpdatedAt = common.GetTimestamp()
	return nil
}

func (s *TokenPoolSubscription) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatedAt = common.GetTimestamp()
	return nil
}

func GetTokenPoolSubscriptionOrderByTradeNo(tradeNo string) (*TokenPoolSubscriptionOrder, error) {
	if tradeNo == "" {
		return nil, errors.New("empty trade_no")
	}
	var o TokenPoolSubscriptionOrder
	err := DB.Where("trade_no = ?", tradeNo).First(&o).Error
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// TokenPoolSubscriptionPendingReuseSeconds is how long a pending checkout QR may be reused.
const TokenPoolSubscriptionPendingReuseSeconds int64 = 2 * 3600

func GetLatestPendingTokenPoolSubscriptionOrder(tokenId, poolId int) (*TokenPoolSubscriptionOrder, error) {
	if tokenId <= 0 || poolId <= 0 {
		return nil, errors.New("invalid token_id or pool_id")
	}
	var o TokenPoolSubscriptionOrder
	err := DB.Where("token_id = ? AND pool_id = ? AND status = ?", tokenId, poolId, common.TopUpStatusPending).
		Order("id DESC").
		First(&o).Error
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ListPendingNativeTokenPoolSubscriptionOrders returns pending native checkout orders for (token_id, pool_id).
func ListPendingNativeTokenPoolSubscriptionOrders(tokenId, poolId int) ([]TokenPoolSubscriptionOrder, error) {
	if tokenId <= 0 || poolId <= 0 {
		return nil, errors.New("invalid token_id or pool_id")
	}
	var orders []TokenPoolSubscriptionOrder
	q := DB.Where("token_id = ? AND pool_id = ? AND status = ?", tokenId, poolId, common.TopUpStatusPending).
		Where("payment_type = ? OR payment_type = ?", "native", "")
	err := q.Order("id DESC").Find(&orders).Error
	if err != nil {
		return nil, err
	}
	return orders, nil
}

// MarkTokenPoolSubscriptionOrderExpired sets a single order row to expired.
func MarkTokenPoolSubscriptionOrderExpired(tradeNo string) error {
	if tradeNo == "" {
		return errors.New("empty trade_no")
	}
	return DB.Model(&TokenPoolSubscriptionOrder{}).
		Where("trade_no = ?", tradeNo).
		Update("status", common.TopUpStatusExpired).Error
}

func GetLatestPendingTokenPoolSubscriptionOrderByPaymentType(tokenId, poolId int, paymentType string) (*TokenPoolSubscriptionOrder, error) {
	if tokenId <= 0 || poolId <= 0 {
		return nil, errors.New("invalid token_id or pool_id")
	}
	var o TokenPoolSubscriptionOrder
	q := DB.Where("token_id = ? AND pool_id = ? AND status = ?", tokenId, poolId, common.TopUpStatusPending)
	// For native orders, also match empty payment_type for backward compatibility
	// with rows created before PaymentType was explicitly set.
	if paymentType == "native" {
		q = q.Where("payment_type = ? OR payment_type = ?", paymentType, "")
	} else {
		q = q.Where("payment_type = ?", paymentType)
	}
	err := q.Order("id DESC").First(&o).Error
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func GetTokenPoolSubscriptionOrderForToken(tradeNo string, tokenId int) (*TokenPoolSubscriptionOrder, error) {
	if tradeNo == "" {
		return nil, errors.New("empty trade_no")
	}
	if tokenId <= 0 {
		return nil, errors.New("invalid token_id")
	}
	var o TokenPoolSubscriptionOrder
	err := DB.Where("trade_no = ? AND token_id = ?", tradeNo, tokenId).First(&o).Error
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ExpirePendingTokenPoolSubscriptionOrders marks pending orders for the pair as expired.
// When paymentType is non-empty, only that rail is expired (native also matches empty
// payment_type for legacy rows). When paymentType is empty, all payment types are expired.
func ExpirePendingTokenPoolSubscriptionOrders(tokenId, poolId int, paymentType, exceptTradeNo string) error {
	return expirePendingTokenPoolSubscriptionOrders(DB, tokenId, poolId, paymentType, exceptTradeNo)
}

func expirePendingTokenPoolSubscriptionOrders(db *gorm.DB, tokenId, poolId int, paymentType, exceptTradeNo string) error {
	if tokenId <= 0 || poolId <= 0 || db == nil {
		return nil
	}
	q := db.Model(&TokenPoolSubscriptionOrder{}).
		Where("token_id = ? AND pool_id = ? AND status = ?", tokenId, poolId, common.TopUpStatusPending)
	if paymentType != "" {
		if paymentType == "native" {
			q = q.Where("payment_type = ? OR payment_type = ?", paymentType, "")
		} else {
			q = q.Where("payment_type = ?", paymentType)
		}
	}
	if exceptTradeNo != "" {
		q = q.Where("trade_no <> ?", exceptTradeNo)
	}
	return q.Update("status", common.TopUpStatusExpired).Error
}

func InsertTokenPoolSubscriptionOrder(o *TokenPoolSubscriptionOrder) error {
	if o == nil {
		return errors.New("order is nil")
	}
	if o.CreateTime == 0 {
		o.CreateTime = common.GetTimestamp()
	}
	if o.Status == "" {
		o.Status = common.TopUpStatusPending
	}
	return DB.Create(o).Error
}

// GetTokenPoolSubscription loads the subscription row for (token_id, pool_id).
func GetTokenPoolSubscription(tokenId, poolId int) (*TokenPoolSubscription, error) {
	if tokenId <= 0 || poolId <= 0 {
		return nil, errors.New("invalid token_id or pool_id")
	}
	var sub TokenPoolSubscription
	err := DB.Where("token_id = ? AND pool_id = ?", tokenId, poolId).First(&sub).Error
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// TokenHasActivePoolSubscription returns true if token has an active paid window for the pool.
func TokenHasActivePoolSubscription(tokenId, poolId int) (bool, error) {
	if tokenId <= 0 || poolId <= 0 {
		return false, nil
	}
	now := common.GetTimestamp()
	var n int64
	err := DB.Model(&TokenPoolSubscription{}).
		Where("token_id = ? AND pool_id = ? AND period_end >= ?", tokenId, poolId, now).
		Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// GrantFirstRequestTrialIfEligible creates a one-time free subscription window for (tokenId, poolId)
// iff no TokenPoolSubscription row exists yet. The window anchors on now (Asia/Shanghai calendar),
// and period_end lands at 23:59:59 CST on the day now+periodSeconds days.
//
// Returns (true, nil) when a new trial row was created, (false, nil) when a row already existed
// (either an active/expired trial, an admin comp, or a paid window). Concurrent callers race
// through an INSERT ... ON CONFLICT DO NOTHING against uk_tp_token_pool; only the winner gets
// (true, nil) and losers get (false, nil).
func GrantFirstRequestTrialIfEligible(tokenId, poolId int, periodSeconds int64) (bool, error) {
	if tokenId <= 0 || poolId <= 0 {
		return false, errors.New("invalid token_id or pool_id")
	}
	if periodSeconds <= 0 {
		periodSeconds = 30 * secondsPerDay
	}
	// Fast path: if a row already exists (active or expired), the customer has
	// already consumed their free window and we must not extend it here.
	var existing TokenPoolSubscription
	err := DB.Where("token_id = ? AND pool_id = ?", tokenId, poolId).First(&existing).Error
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	now := common.GetTimestamp()
	newEnd := periodEndAtBillingEOD(now, periodSeconds)
	sub := TokenPoolSubscription{
		TokenId:     tokenId,
		PoolId:      poolId,
		PeriodStart: now,
		PeriodEnd:   newEnd,
		LastOrderId: 0,
	}
	// Race-safe insert: on unique-index conflict against uk_tp_token_pool the
	// concurrent winner's row survives and this call reports not-granted.
	res := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "token_id"}, {Name: "pool_id"}},
		DoNothing: true,
	}).Create(&sub)
	if res.Error != nil {
		if isUniqueConstraintViolation(res.Error) {
			return false, nil
		}
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

var poolSubscriptionLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}()

const secondsPerDay int64 = 86400

// periodEndAtBillingEOD returns unix seconds for 23:59:59 on the expiry calendar day
// (anchor local date + billing period in whole days) in Asia/Shanghai.
func periodEndAtBillingEOD(anchorUnix, periodSeconds int64) int64 {
	if periodSeconds < secondsPerDay {
		periodSeconds = secondsPerDay
	}
	periodDays := periodSeconds / secondsPerDay
	anchor := time.Unix(anchorUnix, 0).In(poolSubscriptionLocation)
	y, m, d := anchor.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, poolSubscriptionLocation)
	expiryDay := midnight.AddDate(0, 0, int(periodDays))
	endOfDay := time.Date(expiryDay.Year(), expiryDay.Month(), expiryDay.Day(), 23, 59, 59, 0, poolSubscriptionLocation)
	return endOfDay.Unix()
}

// CompleteTokenPoolSubscriptionFromNotify marks the order paid (once) and extends subscription.
func CompleteTokenPoolSubscriptionFromNotify(tradeNo, wechatTxnId, rawJSON string, amountTotal int64, currency string) error {
	if tradeNo == "" {
		return errors.New("empty trade_no")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var order TokenPoolSubscriptionOrder
		if err := tx.Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
			return err
		}
		if order.Status == common.TopUpStatusSuccess {
			return nil
		}
		if !isTokenPoolSubscriptionOrderFulfillable(order.Status) {
			return fmt.Errorf("order not fulfillable: %s", order.Status)
		}
		if amountTotal > 0 && order.AmountTotalFen > 0 && amountTotal != order.AmountTotalFen {
			return fmt.Errorf("%w: amount mismatch: want %d got %d", ErrPoolSubscriptionOrderUnfulfillable, order.AmountTotalFen, amountTotal)
		}
		if currency != "" && order.Currency != "" && currency != order.Currency {
			return fmt.Errorf("%w: currency mismatch", ErrPoolSubscriptionOrderUnfulfillable)
		}
		now := common.GetTimestamp()
		if err := tx.Model(&order).Updates(map[string]interface{}{
			"status":                common.TopUpStatusSuccess,
			"wechat_transaction_id": wechatTxnId,
			"raw_notify":            rawJSON,
			"complete_time":         now,
		}).Error; err != nil {
			return err
		}
		// Invalidate sibling pending checkouts (other payment types / superseded QR/JSAPI).
		if err := expirePendingTokenPoolSubscriptionOrders(tx, order.TokenId, order.PoolId, "", order.TradeNo); err != nil {
			return err
		}
		periodMonths := order.PeriodMonths
		if periodMonths <= 0 {
			periodMonths = 1
		}
		totalPeriodSeconds := order.BillingPeriodSeconds * int64(periodMonths)
		if order.IsUpgrade && order.UpgradedFromPoolId > 0 && order.UpgradedFromPoolId != order.PoolId {
			return upgradeTokenPoolSubscriptionTx(tx, order.TokenId, order.UpgradedFromPoolId, order.PoolId, order.Id, totalPeriodSeconds, order.CreditSecondsGranted, now)
		}
		return upsertTokenPoolSubscriptionTx(tx, order.TokenId, order.PoolId, order.Id, totalPeriodSeconds, now, 0)
	})
}

func upsertTokenPoolSubscriptionTx(tx *gorm.DB, tokenId, poolId, orderId int, periodSeconds int64, now int64, extraCreditSeconds int64) error {
	var sub TokenPoolSubscription
	err := tx.Where("token_id = ? AND pool_id = ?", tokenId, poolId).First(&sub).Error
	base := now
	if err == nil {
		if sub.PeriodEnd > base {
			base = sub.PeriodEnd
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	effectiveSeconds := periodSeconds
	if extraCreditSeconds > 0 {
		effectiveSeconds += extraCreditSeconds
	}
	newEnd := periodEndAtBillingEOD(base, effectiveSeconds)
	if sub.Id == 0 {
		sub = TokenPoolSubscription{
			TokenId:     tokenId,
			PoolId:      poolId,
			PeriodStart: now,
			PeriodEnd:   newEnd,
			LastOrderId: orderId,
		}
		return tx.Create(&sub).Error
	}
	return tx.Model(&sub).Updates(map[string]interface{}{
		"period_end":    newEnd,
		"last_order_id": orderId,
	}).Error
}

// upgradeTokenPoolSubscriptionTx closes the old plan-group subscription (period_end = now - 1)
// and upserts the new pool's subscription with the paid period plus prorated credit seconds.
func upgradeTokenPoolSubscriptionTx(tx *gorm.DB, tokenId, fromPoolId, toPoolId, orderId int, periodSeconds int64, creditSeconds int64, now int64) error {
	if tokenId <= 0 || fromPoolId <= 0 || toPoolId <= 0 || fromPoolId == toPoolId {
		return fmt.Errorf("invalid upgrade parameters")
	}
	var oldSub TokenPoolSubscription
	err := tx.Where("token_id = ? AND pool_id = ?", tokenId, fromPoolId).First(&oldSub).Error
	if err == nil {
		// Only shorten the window when the old one is still active; leave already-expired rows alone.
		if oldSub.PeriodEnd >= now {
			if err := tx.Model(&oldSub).Updates(map[string]interface{}{
				"period_end": now - 1,
			}).Error; err != nil {
				return err
			}
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return upsertTokenPoolSubscriptionTx(tx, tokenId, toPoolId, orderId, periodSeconds, now, creditSeconds)
}

// UpgradeTokenPoolSubscriptionTx is the exported wrapper for admin/manual paths that need to
// close an old plan-group subscription and open a new one atomically.
func UpgradeTokenPoolSubscriptionTx(tx *gorm.DB, tokenId, fromPoolId, toPoolId, orderId int, periodSeconds int64, creditSeconds int64, now int64) error {
	return upgradeTokenPoolSubscriptionTx(tx, tokenId, fromPoolId, toPoolId, orderId, periodSeconds, creditSeconds, now)
}

func ListTokenPoolSubscriptionOrders(offset, limit int) ([]*TokenPoolSubscriptionOrder, int64, error) {
	var items []*TokenPoolSubscriptionOrder
	var total int64
	q := DB.Model(&TokenPoolSubscriptionOrder{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	err := q.Order("id DESC").Find(&items).Error
	return items, total, err
}

// ListTokenPoolSubscriptions returns subscription rows with optional token_id / pool_id / name filters.
func ListTokenPoolSubscriptions(offset, limit, tokenIdFilter, poolIdFilter int, tokenNameFilter, poolNameFilter string) ([]*TokenPoolSubscription, int64, error) {
	q := DB.Model(&TokenPoolSubscription{})
	if tokenIdFilter > 0 {
		q = q.Where("token_id = ?", tokenIdFilter)
	}
	if poolIdFilter > 0 {
		q = q.Where("pool_id = ?", poolIdFilter)
	}
	tokenNameFilter = strings.TrimSpace(tokenNameFilter)
	if tokenNameFilter != "" {
		pattern := "%" + tokenNameFilter + "%"
		tokenIds := make([]int, 0)
		if err := DB.Model(&Token{}).Where("name LIKE ?", pattern).Pluck("id", &tokenIds).Error; err != nil {
			return nil, 0, err
		}
		if len(tokenIds) == 0 {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("token_id IN ?", tokenIds)
		}
	}
	poolNameFilter = strings.TrimSpace(poolNameFilter)
	if poolNameFilter != "" {
		pattern := "%" + poolNameFilter + "%"
		poolIds := make([]int, 0)
		if err := DB.Model(&Pool{}).Where("name LIKE ?", pattern).Pluck("id", &poolIds).Error; err != nil {
			return nil, 0, err
		}
		if len(poolIds) == 0 {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("pool_id IN ?", poolIds)
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	var items []*TokenPoolSubscription
	err := q.Order("id DESC").Find(&items).Error
	return items, total, err
}

// AdminUpsertTokenPoolSubscription sets absolute period_start / period_end for (token_id, pool_id).
// Manual grants use last_order_id = 0 on create; existing last_order_id is preserved on update.
func AdminUpsertTokenPoolSubscription(tokenId, poolId int, periodStart, periodEnd int64) (*TokenPoolSubscription, error) {
	if tokenId <= 0 || poolId <= 0 {
		return nil, errors.New("invalid token_id or pool_id")
	}
	if periodEnd <= 0 {
		return nil, errors.New("invalid period_end")
	}
	if _, err := GetTokenById(tokenId); err != nil {
		return nil, fmt.Errorf("token not found: %w", err)
	}
	if _, err := GetPoolById(poolId); err != nil {
		return nil, fmt.Errorf("pool not found: %w", err)
	}

	now := common.GetTimestamp()
	var result TokenPoolSubscription
	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub TokenPoolSubscription
		err := tx.Where("token_id = ? AND pool_id = ?", tokenId, poolId).First(&sub).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			start := periodStart
			if start <= 0 {
				start = now
			}
			sub = TokenPoolSubscription{
				TokenId:     tokenId,
				PoolId:      poolId,
				PeriodStart: start,
				PeriodEnd:   periodEnd,
				LastOrderId: 0,
			}
			if err := tx.Create(&sub).Error; err != nil {
				return err
			}
			result = sub
			return nil
		}
		if err != nil {
			return err
		}
		updates := map[string]interface{}{
			"period_end": periodEnd,
		}
		if periodStart > 0 {
			updates["period_start"] = periodStart
		}
		if err := tx.Model(&sub).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("token_id = ? AND pool_id = ?", tokenId, poolId).First(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
