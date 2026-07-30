package service

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/wechatpay"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"gorm.io/gorm"
)

var (
	poolSubWechatpayClientFunc              = wechatpay.Client
	poolSubQueryTransactionByOutTradeNoFunc = wechatpay.QueryTransactionByOutTradeNo
)

// SetTokenPoolSubscriptionReconcileTestHooks overrides reconcile dependencies (tests only).
func SetTokenPoolSubscriptionReconcileTestHooks(
	clientFn func(context.Context) (*core.Client, *wechatpay.Config, error),
	queryFn func(context.Context, *wechatpay.Config, *core.Client, string) (*payments.Transaction, error),
) func() {
	prevClient := poolSubWechatpayClientFunc
	prevQuery := poolSubQueryTransactionByOutTradeNoFunc
	if clientFn != nil {
		poolSubWechatpayClientFunc = clientFn
	}
	if queryFn != nil {
		poolSubQueryTransactionByOutTradeNoFunc = queryFn
	}
	return func() {
		poolSubWechatpayClientFunc = prevClient
		poolSubQueryTransactionByOutTradeNoFunc = prevQuery
	}
}

func isTokenPoolSubscriptionOrderReconcilable(status string) bool {
	return status == common.TopUpStatusPending || status == common.TopUpStatusExpired
}

// ReconcileTokenPoolSubscriptionOrderFromWeChat queries WeChat for order state and fulfills or expires locally.
func ReconcileTokenPoolSubscriptionOrderFromWeChat(ctx context.Context, order *model.TokenPoolSubscriptionOrder) (bool, error) {
	if order == nil || !isTokenPoolSubscriptionOrderReconcilable(order.Status) {
		return false, nil
	}
	client, cfg, err := poolSubWechatpayClientFunc(ctx)
	if err != nil || client == nil || cfg == nil {
		return false, err
	}
	tx, err := poolSubQueryTransactionByOutTradeNoFunc(ctx, cfg, client, order.TradeNo)
	if err != nil {
		return false, err
	}
	if tx == nil || tx.TradeState == nil {
		return false, nil
	}
	state := *tx.TradeState
	switch state {
	case "SUCCESS":
		if err := FulfillTokenPoolSubscriptionFromTransaction(tx); err != nil {
			return false, err
		}
		return true, nil
	case "CLOSED", "REVOKED":
		_ = model.MarkTokenPoolSubscriptionOrderExpired(order.TradeNo)
		order.Status = common.TopUpStatusExpired
		return false, nil
	default:
		return false, nil
	}
}

// FulfillTokenPoolSubscriptionFromTransaction completes a paid pool subscription from a WeChat transaction.
func FulfillTokenPoolSubscriptionFromTransaction(tx *payments.Transaction) error {
	if tx == nil || tx.OutTradeNo == nil {
		return errors.New("missing transaction")
	}
	outNo := *tx.OutTradeNo
	var total int64
	if tx.Amount != nil && tx.Amount.Total != nil {
		total = *tx.Amount.Total
	}
	cur := "CNY"
	if tx.Amount != nil && tx.Amount.Currency != nil {
		cur = *tx.Amount.Currency
	}
	wxTxn := ""
	if tx.TransactionId != nil {
		wxTxn = *tx.TransactionId
	}
	raw, _ := common.Marshal(tx)
	LockOrder(outNo)
	defer UnlockOrder(outNo)
	return model.CompleteTokenPoolSubscriptionFromNotify(outNo, wxTxn, string(raw), total, cur)
}

// MaybeReconcilePendingPoolSubscription queries WeChat when a pending order exists for (tokenId, poolId).
// Returns true when fulfillment ran successfully.
func MaybeReconcilePendingPoolSubscription(ctx context.Context, tokenId, poolId int) (bool, error) {
	if tokenId <= 0 || poolId <= 0 {
		return false, nil
	}
	order, err := model.GetLatestPendingTokenPoolSubscriptionOrder(tokenId, poolId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return ReconcileTokenPoolSubscriptionOrderFromWeChat(ctx, order)
}
