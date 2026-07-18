package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/wechatpay"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type jsapiCheckoutRequest struct {
	PoolId       int    `json:"pool_id"`
	PeriodMonths *int   `json:"period_months,omitempty"`
	Code         string `json:"code"`
	PageURL      string `json:"page_url"`
}

type jsapiConfigResponse struct {
	AppId     string `json:"appId"`
	Timestamp int64  `json:"timeStamp"`
	NonceStr  string `json:"nonceStr"`
	Package   string `json:"package"`
	SignType  string `json:"signType"`
	PaySign   string `json:"paySign"`
}

type jsapiCheckoutResponse struct {
	PrepayId       string             `json:"prepay_id"`
	JsapiPayParams jsapiConfigResponse `json:"jsapi_pay_params"`
	WxConfig       wxConfigResponse   `json:"wx_config"`
	PoolName       string             `json:"pool_name"`
	AmountFen      int64              `json:"amount_fen"`
	BaseAmountFen  int64              `json:"base_amount_fen"`
	PeriodMonths   int                `json:"period_months"`
	TradeNo        string             `json:"trade_no"`
	Status         string             `json:"status"`
}

type wxConfigResponse struct {
	AppId     string `json:"appId"`
	Timestamp int64  `json:"timestamp"`
	NonceStr  string `json:"nonceStr"`
	Signature string `json:"signature"`
}

func WechatPayJsapiAppid(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId <= 0 {
		common.ApiErrorMsg(c, "invalid token")
		return
	}

	ctx := c.Request.Context()
	_, cfg, err := wechatpay.Client(ctx)
	if err != nil || cfg == nil {
		common.ApiErrorMsg(c, "wechat pay is not configured")
		return
	}

	appID, err := wechatpay.GetJsapiAppID(cfg)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	common.ApiSuccess(c, gin.H{
		"appid": appID,
	})
}

func WechatPayJsapiCheckout(c *gin.Context) {
	var req jsapiCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PoolId <= 0 {
		common.ApiErrorMsg(c, "invalid request: pool_id required")
		return
	}
	if req.Code == "" {
		common.ApiErrorMsg(c, "invalid request: code required")
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
		common.ApiErrorMsg(c, "pool_id must match the resolved pool for this token")
		return
	}

	if !model.PoolRequiresPaidSubscription(resolved) {
		common.ApiErrorMsg(c, "pool has no monthly subscription price")
		return
	}

	periodMonths := 1
	if req.PeriodMonths != nil && *req.PeriodMonths > 0 {
		periodMonths = *req.PeriodMonths
	}
	options, err := getPoolPeriodOptions(resolved.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	option, ok := findPeriodOption(options, periodMonths)
	if !ok {
		common.ApiErrorMsg(c, "period_months not offered for this pool")
		return
	}

	amountFen := computeAmountFen(resolved.MonthlyPriceCny, option.PeriodMonths, option.DiscountRatioBp)
	if amountFen <= 0 {
		common.ApiErrorMsg(c, "invalid pool price")
		return
	}

	ctx := c.Request.Context()
	client, cfg, err := wechatpay.Client(ctx)
	if err != nil || client == nil || cfg == nil {
		common.ApiErrorMsg(c, "wechat pay is not configured")
		return
	}

	if !wechatpay.IsJsapiConfigured(cfg) {
		common.ApiErrorMsg(c, "wechat pay jsapi is not configured")
		return
	}

	openid, err := wechatpay.GetOpenid(cfg.AppID, cfg.AppSecret, req.Code)
	if err != nil {
		common.ApiErrorMsg(c, "failed to get openid: "+err.Error())
		return
	}

	notifyURL := service.GetCallbackAddress() + "/api/payment/wechat/notify"
	tradeNo := genTokenPoolSubscriptionTradeNo()
	desc := fmt.Sprintf("Pool subscription: %s", poolDisplayName(resolved))

	prepayID, err := wechatpay.JsapiPrepay(ctx, cfg, client, notifyURL, tradeNo, desc, amountFen, openid)
	if err != nil {
		common.ApiErrorMsg(c, "failed to create wechat pay order: "+err.Error())
		return
	}

	_ = model.ExpirePendingTokenPoolSubscriptionOrders(tokenId, resolved.Id, "")

	order := &model.TokenPoolSubscriptionOrder{
		UserId:               userId,
		TokenId:              tokenId,
		PoolId:               resolved.Id,
		AmountCny:            resolved.MonthlyPriceCny,
		AmountTotalFen:       amountFen,
		Currency:             "CNY",
		BillingPeriodSeconds: resolved.BillingPeriodSeconds,
		PeriodMonths:         option.PeriodMonths,
		DiscountRatioBp:      option.DiscountRatioBp,
		TradeNo:              tradeNo,
		PrepayId:             prepayID,
		PaymentType:          "jsapi",
		Openid:               openid,
		Status:               common.TopUpStatusPending,
	}
	if err := model.InsertTokenPoolSubscriptionOrder(order); err != nil {
		common.ApiErrorMsg(c, "failed to persist order")
		return
	}

	timestamp, nonceStr, packageStr, signType, paySign, err := wechatpay.GenerateJsapiPayParams(cfg.AppID, prepayID, cfg.PrivateKey)
	if err != nil {
		common.ApiErrorMsg(c, "failed to generate pay params")
		return
	}

	pageURL := strings.TrimSpace(req.PageURL)
	if pageURL == "" {
		pageURL = service.GetCallbackAddress() + "/wechat-pay-800claw"
	}
	wxConfigTimestamp := timestamp
	wxConfigNonceStr := wechatpay.SnapNonceStr()
	wxSignature, err := wechatpay.GenerateJsapiConfig(cfg.AppID, cfg.AppSecret, wxConfigTimestamp, wxConfigNonceStr, pageURL)
	if err != nil {
		common.ApiErrorMsg(c, "failed to generate wx.config signature")
		return
	}

	baseAmountFen := computeAmountFen(resolved.MonthlyPriceCny, option.PeriodMonths, 10000)

	common.ApiSuccess(c, jsapiCheckoutResponse{
		PrepayId: prepayID,
		JsapiPayParams: jsapiConfigResponse{
			AppId:     cfg.AppID,
			Timestamp: timestamp,
			NonceStr:  nonceStr,
			Package:   packageStr,
			SignType:  signType,
			PaySign:   paySign,
		},
		WxConfig: wxConfigResponse{
			AppId:     cfg.AppID,
			Timestamp: wxConfigTimestamp,
			NonceStr:  wxConfigNonceStr,
			Signature: wxSignature,
		},
		PoolName:      poolDisplayName(resolved),
		AmountFen:     amountFen,
		BaseAmountFen: baseAmountFen,
		PeriodMonths:  periodMonths,
		TradeNo:       tradeNo,
		Status:        common.TopUpStatusPending,
	})
}