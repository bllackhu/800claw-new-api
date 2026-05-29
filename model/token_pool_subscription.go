package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// TokenPoolSubscriptionOrder records a native WeChat Pay pool subscription checkout.
type TokenPoolSubscriptionOrder struct {
	Id                   int     `json:"id"`
	UserId               int     `json:"user_id" gorm:"index"`
	TokenId              int     `json:"token_id" gorm:"index:idx_tp_sub_order_token_pool,priority:1"`
	PoolId               int     `json:"pool_id" gorm:"index:idx_tp_sub_order_token_pool,priority:2"`
	AmountCny            float64 `json:"amount_cny"`
	AmountTotalFen       int64   `json:"amount_total_fen" gorm:"bigint"`
	Currency             string  `json:"currency" gorm:"type:varchar(8);default:'CNY'"`
	BillingPeriodSeconds int64   `json:"billing_period_seconds" gorm:"bigint"`
	TradeNo              string  `json:"trade_no" gorm:"type:varchar(64);uniqueIndex"`
	CodeUrl              string  `json:"-" gorm:"type:text"`
	WechatTransactionId  string  `json:"wechat_transaction_id" gorm:"type:varchar(64);default:''"`
	Status               string  `json:"status" gorm:"type:varchar(32);index"`
	RawNotify            string  `json:"raw_notify" gorm:"type:text"`
	CreateTime           int64   `json:"create_time" gorm:"bigint;index"`
	CompleteTime         int64   `json:"complete_time" gorm:"bigint"`
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

// ExpirePendingTokenPoolSubscriptionOrders marks other pending orders for the pair as expired.
func ExpirePendingTokenPoolSubscriptionOrders(tokenId, poolId int, exceptTradeNo string) error {
	if tokenId <= 0 || poolId <= 0 {
		return nil
	}
	q := DB.Model(&TokenPoolSubscriptionOrder{}).
		Where("token_id = ? AND pool_id = ? AND status = ?", tokenId, poolId, common.TopUpStatusPending)
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
		if order.Status != common.TopUpStatusPending {
			return fmt.Errorf("order not pending: %s", order.Status)
		}
		if amountTotal > 0 && order.AmountTotalFen > 0 && amountTotal != order.AmountTotalFen {
			return fmt.Errorf("amount mismatch: want %d got %d", order.AmountTotalFen, amountTotal)
		}
		if currency != "" && order.Currency != "" && currency != order.Currency {
			return fmt.Errorf("currency mismatch")
		}
		now := common.GetTimestamp()
		if err := tx.Model(&order).Updates(map[string]interface{}{
			"status":                 common.TopUpStatusSuccess,
			"wechat_transaction_id": wechatTxnId,
			"raw_notify":             rawJSON,
			"complete_time":          now,
		}).Error; err != nil {
			return err
		}
		return upsertTokenPoolSubscriptionTx(tx, order.TokenId, order.PoolId, order.Id, order.BillingPeriodSeconds, now)
	})
}

func upsertTokenPoolSubscriptionTx(tx *gorm.DB, tokenId, poolId, orderId int, periodSeconds int64, now int64) error {
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
	newEnd := periodEndAtBillingEOD(base, periodSeconds)
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

// ListTokenPoolSubscriptions returns subscription rows with optional token_id / pool_id filters.
func ListTokenPoolSubscriptions(offset, limit, tokenIdFilter, poolIdFilter int) ([]*TokenPoolSubscription, int64, error) {
	q := DB.Model(&TokenPoolSubscription{})
	if tokenIdFilter > 0 {
		q = q.Where("token_id = ?", tokenIdFilter)
	}
	if poolIdFilter > 0 {
		q = q.Where("pool_id = ?", poolIdFilter)
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
