package model

import (
	"errors"
	"fmt"

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
	newEnd := base + periodSeconds
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
