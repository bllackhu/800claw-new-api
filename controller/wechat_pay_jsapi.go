package controller

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/wechatpay"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type jsapiCheckoutRequest struct {
	PoolId            int    `json:"pool_id"`
	PeriodMonths      *int   `json:"period_months,omitempty"`
	State             string `json:"state"`
	Code              string `json:"code"`
	PageURL           string `json:"page_url"`
	UpgradeFromPoolId *int   `json:"upgrade_from_pool_id,omitempty"`
}

type jsapiConfigResponse struct {
	AppId     string `json:"appId"`
	Timestamp string `json:"timeStamp"`
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
	Timestamp string `json:"timestamp"`
	NonceStr  string `json:"nonceStr"`
	Signature string `json:"signature"`
}

// WechatPayJsapiOrderQuery is a public endpoint for querying a JSAPI order by trade_no.
// It triggers reconciliation from WeChat so the caller can poll until success.
func WechatPayJsapiOrderQuery(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Query("trade_no"))
	if tradeNo == "" {
		common.ApiErrorMsg(c, "trade_no required")
		return
	}

	order, err := model.GetTokenPoolSubscriptionOrderByTradeNo(tradeNo)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "order not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	if order.PaymentType != "jsapi" {
		common.ApiErrorMsg(c, "invalid order type")
		return
	}

	reconciled, reconcileErr := reconcileTokenPoolSubscriptionOrderFromWeChat(c.Request.Context(), order)
	if reconcileErr != nil {
		logger.LogError(c, "wechat jsapi order query failed trade_no="+tradeNo+": "+reconcileErr.Error())
	}

	if reconciled {
		order, err = model.GetTokenPoolSubscriptionOrderByTradeNo(tradeNo)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}

	poolName := poolDisplayNameFallback
	if pool, poolErr := model.GetPoolById(order.PoolId); poolErr == nil && pool != nil {
		poolName = poolDisplayName(pool)
	}

	common.ApiSuccess(c, gin.H{
		"trade_no":               order.TradeNo,
		"status":                 order.Status,
		"amount_fen":             order.AmountTotalFen,
		"currency":               order.Currency,
		"pool_name":              poolName,
		"period_months":          order.PeriodMonths,
		"complete_time":          order.CompleteTime,
		"reconciled_from_wechat": reconciled,
	})
}

func buildJsapiCheckoutResponse(order *model.TokenPoolSubscriptionOrder, pool *model.Pool, periodMonths int, cfg *wechatpay.Config, pageURL string) (jsapiCheckoutResponse, error) {
	timestamp, nonceStr, packageStr, signType, paySign, err := wechatpay.GenerateJsapiPayParams(cfg.AppID, order.PrepayId, cfg.PrivateKey)
	if err != nil {
		return jsapiCheckoutResponse{}, err
	}

	pageURL = wechatpay.StripTrailingHash(pageURL)
	if pageURL == "" {
		pageURL = getJsapiRedirectURI()
	}
	wxConfigTimestamp := timestamp
	wxConfigNonceStr := wechatpay.SnapNonceStr()
	wxSignature, err := wechatpay.GenerateJsapiConfig(cfg.AppID, cfg.AppSecret, wxConfigTimestamp, wxConfigNonceStr, pageURL)
	if err != nil {
		return jsapiCheckoutResponse{}, err
	}

	baseAmountFen := computeAmountFen(pool.MonthlyPriceCny, periodMonths, 10000)
	return jsapiCheckoutResponse{
		PrepayId: order.PrepayId,
		JsapiPayParams: jsapiConfigResponse{
			AppId:     cfg.AppID,
			Timestamp: strconv.FormatInt(timestamp, 10),
			NonceStr:  nonceStr,
			Package:   packageStr,
			SignType:  signType,
			PaySign:   paySign,
		},
		WxConfig: wxConfigResponse{
			AppId:     cfg.AppID,
			Timestamp: strconv.FormatInt(wxConfigTimestamp, 10),
			NonceStr:  wxConfigNonceStr,
			Signature: wxSignature,
		},
		PoolName:      poolDisplayName(pool),
		AmountFen:     order.AmountTotalFen,
		BaseAmountFen: baseAmountFen,
		PeriodMonths:  periodMonths,
		TradeNo:       order.TradeNo,
		Status:        order.Status,
	}, nil
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

func getJsapiRedirectURI() string {
	if uri := strings.TrimSpace(os.Getenv("WECHATPAY_JSAPI_REDIRECT_URI")); uri != "" {
		return uri
	}
	return service.GetCallbackAddress() + "/wechat-pay-800claw/"
}

func WechatPayJsapiOauthParams(c *gin.Context) {
	var req struct {
		PoolId            int `json:"pool_id"`
		PeriodMonths      int `json:"period_months"`
		UpgradeFromPoolId int `json:"upgrade_from_pool_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PoolId <= 0 {
		common.ApiErrorMsg(c, "invalid request: pool_id required")
		return
	}
	if req.PeriodMonths <= 0 {
		req.PeriodMonths = 1
	}

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

	state := generateStateToken(tokenId, req.PoolId, req.PeriodMonths, req.UpgradeFromPoolId)

	redirectURI := getJsapiRedirectURI()
	jsapiOauthPaymentURL := fmt.Sprintf(
		"https://open.weixin.qq.com/connect/oauth2/authorize?appid=%s&redirect_uri=%s&response_type=code&scope=snsapi_base&state=%s#wechat_redirect",
		url.QueryEscape(appID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
	)

	common.ApiSuccess(c, gin.H{
		"appid":                    appID,
		"state":                    state,
		"jsapi_oauth_payment_url":  jsapiOauthPaymentURL,
	})
}

func WechatPayJsapiCheckout(c *gin.Context) {
	var req jsapiCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	if req.Code == "" {
		common.ApiErrorMsg(c, "invalid request: code required")
		return
	}

	var tokenId, userId, poolId, periodMonths, upgradeFromPoolId int

	if req.State != "" {
		var ok bool
		tokenId, poolId, periodMonths, upgradeFromPoolId, ok = consumeStateToken(req.State)
		if !ok {
			common.ApiErrorMsg(c, "invalid or expired state token")
			return
		}
	} else {
		poolId = req.PoolId
		periodMonths = 1
		if poolId <= 0 {
			common.ApiErrorMsg(c, "invalid request: pool_id or state required")
			return
		}
		if req.PeriodMonths != nil && *req.PeriodMonths > 0 {
			periodMonths = *req.PeriodMonths
		}
		if req.UpgradeFromPoolId != nil && *req.UpgradeFromPoolId > 0 {
			upgradeFromPoolId = *req.UpgradeFromPoolId
		}
		tokenId = c.GetInt("token_id")
		userId = c.GetInt("id")
		if tokenId <= 0 || userId <= 0 {
			common.ApiErrorMsg(c, "invalid token context")
			return
		}
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

	if req.State == "" && token.UserId != userId {
		common.ApiErrorMsg(c, "token not found")
		return
	}
	userId = token.UserId

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
	if poolId != resolved.Id {
		// Allow explicit upgrade purchases to a different pool in the same plan_group with a higher plan_tier.
		if upgradeFromPoolId == 0 || upgradeFromPoolId != resolved.Id {
			common.ApiErrorMsg(c, "pool_id must match the resolved pool for this token or be an upgrade target")
			return
		}
	}

	// Determine target pool: either the resolved pool (renew) or an explicit upgrade target.
	targetPool := resolved
	if poolId != resolved.Id {
		target, err := model.GetPoolById(poolId)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				common.ApiErrorMsg(c, "target pool not found")
				return
			}
			common.ApiError(c, err)
			return
		}
		if target == nil || target.Status != model.PoolStatusEnabled {
			common.ApiErrorMsg(c, "target pool is not available")
			return
		}
		if strings.TrimSpace(target.PlanGroup) == "" || target.PlanGroup != resolved.PlanGroup {
			common.ApiErrorMsg(c, "target pool is not in the same plan_group as the current pool")
			return
		}
		if target.PlanTier <= resolved.PlanTier {
			common.ApiErrorMsg(c, "target pool must be a higher tier than the current pool")
			return
		}
		targetPool = target
	}

	if !model.PoolRequiresPaidSubscription(targetPool) {
		common.ApiErrorMsg(c, "pool has no monthly subscription price")
		return
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

	amountFen := computeAmountFen(targetPool.MonthlyPriceCny, option.PeriodMonths, option.DiscountRatioBp)
	if amountFen <= 0 {
		common.ApiErrorMsg(c, "invalid pool price")
		return
	}

	// Compute upgrade credit from remaining old-pool time (only when this is an upgrade path).
	isUpgrade := upgradeFromPoolId > 0 && upgradeFromPoolId != targetPool.Id
	var creditSeconds int64
	if isUpgrade {
		fromPool, err := model.GetPoolById(upgradeFromPoolId)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				common.ApiErrorMsg(c, "upgrade_from_pool_id not found")
				return
			}
			common.ApiError(c, err)
			return
		}
		if fromPool == nil {
			common.ApiErrorMsg(c, "upgrade_from_pool_id not found")
			return
		}
		if strings.TrimSpace(fromPool.PlanGroup) == "" || fromPool.PlanGroup != targetPool.PlanGroup {
			common.ApiErrorMsg(c, "upgrade_from_pool_id must share the same plan_group as the target pool")
			return
		}
		if fromPool.PlanTier >= targetPool.PlanTier {
			common.ApiErrorMsg(c, "upgrade_from_pool_id must be a lower tier than the target pool")
			return
		}
		fromSub, err := model.GetTokenPoolSubscription(tokenId, upgradeFromPoolId)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				common.ApiError(c, err)
				return
			}
		}
		nowTs := common.GetTimestamp()
		if fromSub != nil && fromSub.PeriodEnd > nowTs {
			creditSeconds = computeUpgradeCreditSeconds(fromSub.PeriodEnd-nowTs, fromPool.MonthlyPriceCny, targetPool.MonthlyPriceCny)
		}
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

	// Reuse a recent pending JSAPI order if it matches exactly.
	now := common.GetTimestamp()
	if pending, err := model.GetLatestPendingTokenPoolSubscriptionOrderByPaymentType(tokenId, targetPool.Id, "jsapi"); err == nil && pending != nil {
		if pending.AmountTotalFen == amountFen &&
			pending.PeriodMonths == option.PeriodMonths &&
			pending.IsUpgrade == isUpgrade &&
			pending.UpgradedFromPoolId == upgradeFromPoolId &&
			pending.PrepayId != "" &&
			now-pending.CreateTime <= model.TokenPoolSubscriptionPendingReuseSeconds {
			resp, buildErr := buildJsapiCheckoutResponse(pending, targetPool, option.PeriodMonths, cfg, req.PageURL)
			if buildErr != nil {
				common.ApiErrorMsg(c, "failed to build checkout response")
				return
			}
			common.ApiSuccess(c, resp)
			return
		}
	}

	openid, err := wechatpay.GetOpenid(cfg.AppID, cfg.AppSecret, req.Code)
	if err != nil {
		common.ApiErrorMsg(c, "failed to get openid: "+err.Error())
		return
	}

	notifyURL := service.GetCallbackAddress() + "/api/payment/wechat/notify"
	tradeNo := genTokenPoolSubscriptionTradeNo()
	desc := fmt.Sprintf("Pool subscription: %s", poolDisplayName(targetPool))

	prepayID, err := wechatpay.JsapiPrepay(ctx, cfg, client, notifyURL, tradeNo, desc, amountFen, openid)
	if err != nil {
		common.ApiErrorMsg(c, "failed to create wechat pay order: "+err.Error())
		return
	}

	_ = model.ExpirePendingTokenPoolSubscriptionOrders(tokenId, targetPool.Id, "jsapi", "")

	order := &model.TokenPoolSubscriptionOrder{
		UserId:               userId,
		TokenId:              tokenId,
		PoolId:               targetPool.Id,
		AmountCny:            targetPool.MonthlyPriceCny,
		AmountTotalFen:       amountFen,
		Currency:             "CNY",
		BillingPeriodSeconds: targetPool.BillingPeriodSeconds,
		PeriodMonths:         option.PeriodMonths,
		DiscountRatioBp:      option.DiscountRatioBp,
		IsUpgrade:            isUpgrade,
		UpgradedFromPoolId:   upgradeFromPoolId,
		CreditSecondsGranted: creditSeconds,
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
	pageURL = wechatpay.StripTrailingHash(pageURL)
	if pageURL == "" {
		pageURL = getJsapiRedirectURI()
	}
	wxConfigTimestamp := timestamp
	wxConfigNonceStr := wechatpay.SnapNonceStr()
	wxSignature, err := wechatpay.GenerateJsapiConfig(cfg.AppID, cfg.AppSecret, wxConfigTimestamp, wxConfigNonceStr, pageURL)
	if err != nil {
		common.ApiErrorMsg(c, "failed to generate wx.config signature")
		return
	}

	baseAmountFen := computeAmountFen(targetPool.MonthlyPriceCny, option.PeriodMonths, 10000)

	common.ApiSuccess(c, jsapiCheckoutResponse{
		PrepayId: prepayID,
		JsapiPayParams: jsapiConfigResponse{
			AppId:     cfg.AppID,
			Timestamp: strconv.FormatInt(timestamp, 10),
			NonceStr:  nonceStr,
			Package:   packageStr,
			SignType:  signType,
			PaySign:   paySign,
		},
		WxConfig: wxConfigResponse{
			AppId:     cfg.AppID,
			Timestamp: strconv.FormatInt(wxConfigTimestamp, 10),
			NonceStr:  wxConfigNonceStr,
			Signature: wxSignature,
		},
		PoolName:      poolDisplayName(targetPool),
		AmountFen:     amountFen,
		BaseAmountFen: baseAmountFen,
		PeriodMonths:  periodMonths,
		TradeNo:       tradeNo,
		Status:        common.TopUpStatusPending,
	})
}