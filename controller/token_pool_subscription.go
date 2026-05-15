package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/wechatpay"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type tokenPoolSubscriptionCheckoutRequest struct {
	PoolId int `json:"pool_id"`
}

func genTokenPoolSubscriptionTradeNo() string {
	// WeChat out_trade_no: 6–32 chars, [A-Za-z0-9_*-]
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	if len(suffix) > 12 {
		suffix = suffix[len(suffix)-12:]
	}
	base := "TP" + common.GetRandomString(6) + suffix
	if len(base) > 32 {
		base = base[:32]
	}
	if len(base) < 6 {
		base = base + common.GetRandomString(6)
	}
	return base
}

// RequestTokenPoolSubscriptionWechatCheckout creates a pending order and returns a WeChat Native pay code_url.
// Auth: Bearer sk-... (same as relay). Body: {"pool_id": N} where N must match ResolvePoolForContext for this token.
func RequestTokenPoolSubscriptionWechatCheckout(c *gin.Context) {
	var req tokenPoolSubscriptionCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PoolId <= 0 {
		common.ApiErrorMsg(c, "invalid request: pool_id required")
		return
	}
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	if tokenId <= 0 || userId <= 0 {
		common.ApiErrorMsg(c, "invalid token context")
		return
	}

	token, err := model.GetTokenById(tokenId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "token not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	if token.UserId != userId {
		common.ApiErrorMsg(c, "token not found")
		return
	}

	resolved, err := model.ResolvePoolForContext(token.UserId, token.Id, token.Group)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "no resolved pool for this token")
			return
		}
		common.ApiError(c, err)
		return
	}
	if resolved == nil || resolved.Id <= 0 {
		common.ApiErrorMsg(c, "no resolved pool for this token")
		return
	}
	if req.PoolId != resolved.Id {
		common.ApiErrorMsg(c, "pool_id must match the resolved pool for this token (see GET /api/usage/token/pool)")
		return
	}

	pool := resolved
	if !model.PoolRequiresPaidSubscription(pool) {
		common.ApiErrorMsg(c, "pool has no monthly subscription price")
		return
	}

	amountFen := decimal.NewFromFloat(pool.MonthlyPriceCny).Mul(decimal.NewFromInt(100)).Round(0).IntPart()
	if amountFen <= 0 {
		common.ApiErrorMsg(c, "invalid pool price")
		return
	}

	ctx := c.Request.Context()
	client, cfg, err := wechatpay.Client(ctx)
	if err != nil || client == nil || cfg == nil {
		// Client() fails both when env is incomplete and when core.NewClient fails (e.g. outbound TLS
		// to WeChat, bad key material). Log the real error — the API message stays generic.
		if err != nil {
			logger.LogError(c, "wechatpay client init: "+err.Error())
		} else {
			logger.LogError(c, "wechatpay client init: nil client or config (unexpected)")
		}
		common.ApiErrorMsg(c, "wechat pay is not configured on this server")
		return
	}

	tradeNo := genTokenPoolSubscriptionTradeNo()
	notifyURL := service.GetCallbackAddress() + "/api/payment/wechat/notify"
	desc := fmt.Sprintf("Pool subscription #%d", pool.Id)

	codeURL, err := wechatpay.NativePrepay(ctx, cfg, client, notifyURL, tradeNo, desc, amountFen)
	if err != nil {
		logger.LogError(c, "wechat native prepay failed: "+err.Error())
		common.ApiErrorMsg(c, "failed to create wechat pay order")
		return
	}

	period := pool.BillingPeriodSeconds
	if period <= 0 {
		period = 30 * 24 * 3600
	}
	cur := pool.BillingCurrency
	if cur == "" {
		cur = "CNY"
	}
	order := &model.TokenPoolSubscriptionOrder{
		UserId:               userId,
		TokenId:              tokenId,
		PoolId:               resolved.Id,
		AmountCny:            pool.MonthlyPriceCny,
		AmountTotalFen:       amountFen,
		Currency:             cur,
		BillingPeriodSeconds: period,
		TradeNo:              tradeNo,
		Status:               common.TopUpStatusPending,
	}
	if err := model.InsertTokenPoolSubscriptionOrder(order); err != nil {
		logger.LogError(c, "insert token pool subscription order failed: "+err.Error())
		common.ApiErrorMsg(c, "failed to persist order")
		return
	}

	common.ApiSuccess(c, gin.H{
		"code_url":  codeURL,
		"trade_no":  tradeNo,
		"amount_fen": amountFen,
		"currency":   cur,
	})
}

// WeChatPayPoolSubscriptionNotify handles WeChat Pay v3 payment notifications for pool subscriptions.
func WeChatPayPoolSubscriptionNotify(c *gin.Context) {
	ctx := context.Background()
	_, cfg, err := wechatpay.Client(ctx)
	if err != nil || cfg == nil {
		if err != nil {
			logger.LogError(c, "wechat pay notify: client not available: "+err.Error())
		} else {
			logger.LogError(c, "wechat pay notify: client not available: nil config")
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "FAIL", "message": "not configured"})
		return
	}

	_, tx, err := wechatpay.ParsePaymentNotify(ctx, cfg, c.Request)
	if err != nil {
		logger.LogError(c, "wechat pay notify parse failed: "+err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "invalid notify"})
		return
	}

	if tx == nil || tx.TradeState == nil || *tx.TradeState != "SUCCESS" {
		c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "成功"})
		return
	}
	if tx.OutTradeNo == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "missing out_trade_no"})
		return
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

	if err := model.CompleteTokenPoolSubscriptionFromNotify(outNo, wxTxn, string(raw), total, cur); err != nil {
		logger.LogError(c, "complete token pool subscription failed trade_no="+outNo+" err="+err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"code": "FAIL", "message": "fulfillment error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "成功"})
}

// GetPoolSubscriptionOrders lists token pool subscription orders (admin).
func GetPoolSubscriptionOrders(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListTokenPoolSubscriptionOrders(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
	})
}
