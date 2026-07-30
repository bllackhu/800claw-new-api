package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/wechatpay"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"gorm.io/gorm"
)

var (
	nativePrepayFunc       = wechatpay.NativePrepay
	wechatpayClientFunc    = wechatpay.Client
	closeNativeOrderFunc   = wechatpay.CloseOrderByOutTradeNo
	parsePaymentNotifyFunc = wechatpay.ParsePaymentNotify
)

type tokenPoolSubscriptionCheckoutRequest struct {
	PoolId              int  `json:"pool_id"`
	PeriodMonths        *int `json:"period_months,omitempty"`
	UpgradeFromPoolId   *int `json:"upgrade_from_pool_id,omitempty"`
}

type tokenPoolSubscriptionQuoteRequest struct {
	PoolId            int  `json:"pool_id"`
	PeriodMonths      *int `json:"period_months,omitempty"`
	UpgradeFromPoolId *int `json:"upgrade_from_pool_id,omitempty"`
}

// poolPeriodOptionDTO is the customer-facing representation of a PoolPeriodOption.
type poolPeriodOptionDTO struct {
	PeriodMonths    int   `json:"period_months"`
	DiscountRatioBp int   `json:"discount_ratio_bp"`
	BaseAmountFen   int64 `json:"base_amount_fen"`
	AmountFen       int64 `json:"amount_fen"`
}

// defaultPoolPeriodOptions returns the fallback multi-period lineup when no PoolPeriodOption rows
// are configured for a pool. Values mirror the plan document: 1m/10000, 3m/9600, 6m/9000, 12m/8000.
func defaultPoolPeriodOptions() []model.PoolPeriodOption {
	return []model.PoolPeriodOption{
		{PeriodMonths: 1, DiscountRatioBp: 10000, Enabled: true, SortOrder: 1},
		{PeriodMonths: 3, DiscountRatioBp: 9600, Enabled: true, SortOrder: 2},
		{PeriodMonths: 6, DiscountRatioBp: 9000, Enabled: true, SortOrder: 3},
		{PeriodMonths: 12, DiscountRatioBp: 8000, Enabled: true, SortOrder: 4},
	}
}

// getPoolPeriodOptions returns configured options for a pool, falling back to the defaults when
// none exist. The result is always non-empty and ordered by period_months.
func getPoolPeriodOptions(poolId int) ([]model.PoolPeriodOption, error) {
	items, err := model.GetPoolPeriodOptions(poolId)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return defaultPoolPeriodOptions(), nil
	}
	out := make([]model.PoolPeriodOption, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		out = append(out, *it)
	}
	if len(out) == 0 {
		return defaultPoolPeriodOptions(), nil
	}
	return out, nil
}

// computeAmountFen returns the checkout amount in fen for (monthly_price_cny, period_months, discount_ratio_bp).
// The formula is: monthly_price_cny * 100 (fen) * period_months * discount_ratio_bp / 10000, rounded to fen.
func computeAmountFen(monthlyPriceCny float64, periodMonths, discountRatioBp int) int64 {
	if periodMonths <= 0 || monthlyPriceCny <= 0 {
		return 0
	}
	if discountRatioBp <= 0 {
		discountRatioBp = 10000
	}
	monthlyFen := decimal.NewFromFloat(monthlyPriceCny).Mul(decimal.NewFromInt(100))
	total := monthlyFen.Mul(decimal.NewFromInt(int64(periodMonths))).
		Mul(decimal.NewFromInt(int64(discountRatioBp))).
		Div(decimal.NewFromInt(10000))
	return total.Round(0).IntPart()
}

// findPeriodOption locates the option matching periodMonths from the configured list.
func findPeriodOption(options []model.PoolPeriodOption, periodMonths int) (model.PoolPeriodOption, bool) {
	for _, opt := range options {
		if opt.Enabled && opt.PeriodMonths == periodMonths {
			return opt, true
		}
	}
	return model.PoolPeriodOption{}, false
}

// buildPeriodOptionDTOs materializes options for a given monthly price (rendered on both /plans and /quote).
func buildPeriodOptionDTOs(monthlyPriceCny float64, options []model.PoolPeriodOption) []poolPeriodOptionDTO {
	out := make([]poolPeriodOptionDTO, 0, len(options))
	for _, opt := range options {
		if !opt.Enabled {
			continue
		}
		base := computeAmountFen(monthlyPriceCny, opt.PeriodMonths, 10000)
		amount := computeAmountFen(monthlyPriceCny, opt.PeriodMonths, opt.DiscountRatioBp)
		out = append(out, poolPeriodOptionDTO{
			PeriodMonths:    opt.PeriodMonths,
			DiscountRatioBp: opt.DiscountRatioBp,
			BaseAmountFen:   base,
			AmountFen:       amount,
		})
	}
	return out
}

// computeUpgradeCreditSeconds converts remaining paid seconds on the old pool into equivalent
// paid seconds on the new (more expensive) pool by CNY value:
//   credit_seconds = floor(remaining_seconds * fromMonthly / toMonthly)
// The result is 0 when either pool has zero monthly price or the caller is not currently on the
// old pool.
func computeUpgradeCreditSeconds(remainingSeconds int64, fromMonthly, toMonthly float64) int64 {
	if remainingSeconds <= 0 || fromMonthly <= 0 || toMonthly <= 0 {
		return 0
	}
	ratio := decimal.NewFromFloat(fromMonthly).Div(decimal.NewFromFloat(toMonthly))
	credit := decimal.NewFromInt(remainingSeconds).Mul(ratio).Floor()
	return credit.IntPart()
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
		// Allow explicit upgrade purchases to a different pool in the same plan_group with a higher plan_tier.
		if req.UpgradeFromPoolId == nil || *req.UpgradeFromPoolId != resolved.Id {
			common.ApiErrorMsg(c, "pool_id must match the resolved pool for this token (see GET /api/usage/token/pool) or be an upgrade target")
			return
		}
	}

	// Determine target pool: either the resolved pool (renew) or an explicit upgrade target.
	pool := resolved
	if req.PoolId != resolved.Id {
		target, err := model.GetPoolById(req.PoolId)
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
		pool = target
	}

	if !model.PoolRequiresPaidSubscription(pool) {
		common.ApiErrorMsg(c, "pool has no monthly subscription price")
		return
	}

	periodMonths := 1
	if req.PeriodMonths != nil && *req.PeriodMonths > 0 {
		periodMonths = *req.PeriodMonths
	}
	options, err := getPoolPeriodOptions(pool.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	option, ok := findPeriodOption(options, periodMonths)
	if !ok {
		common.ApiErrorMsg(c, "period_months not offered for this pool")
		return
	}

	amountFen := computeAmountFen(pool.MonthlyPriceCny, option.PeriodMonths, option.DiscountRatioBp)
	if amountFen <= 0 {
		common.ApiErrorMsg(c, "invalid pool price")
		return
	}

	// Compute upgrade credit from remaining Lite time (only when this is an upgrade path).
	isUpgrade := req.UpgradeFromPoolId != nil && *req.UpgradeFromPoolId > 0 && *req.UpgradeFromPoolId != pool.Id
	var upgradeFromPoolId int
	var creditSeconds int64
	if isUpgrade {
		upgradeFromPoolId = *req.UpgradeFromPoolId
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
		if strings.TrimSpace(fromPool.PlanGroup) == "" || fromPool.PlanGroup != pool.PlanGroup {
			common.ApiErrorMsg(c, "upgrade_from_pool_id must share the same plan_group as the target pool")
			return
		}
		if fromPool.PlanTier >= pool.PlanTier {
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
			creditSeconds = computeUpgradeCreditSeconds(fromSub.PeriodEnd-nowTs, fromPool.MonthlyPriceCny, pool.MonthlyPriceCny)
		}
	}

	displayPoolName := poolDisplayName(pool)
	period := pool.BillingPeriodSeconds
	if period <= 0 {
		period = 30 * 24 * 3600
	}
	cur := pool.BillingCurrency
	if cur == "" {
		cur = "CNY"
	}

	now := common.GetTimestamp()
	if pending, err := model.GetLatestPendingTokenPoolSubscriptionOrderByPaymentType(tokenId, pool.Id, "native"); err == nil && pending != nil {
		if pending.AmountTotalFen == amountFen &&
			pending.PeriodMonths == option.PeriodMonths &&
			pending.IsUpgrade == isUpgrade &&
			pending.UpgradedFromPoolId == upgradeFromPoolId &&
			pending.CodeUrl != "" &&
			now-pending.CreateTime <= model.TokenPoolSubscriptionPendingReuseSeconds {
			common.ApiSuccess(c, tokenPoolCheckoutPayload(pending, displayPoolName, true))
			return
		}
	}

	ctx := c.Request.Context()
	client, cfg, err := wechatpayClientFunc(ctx)
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

	notifyURL := service.GetCallbackAddress() + "/api/payment/wechat/notify"
	tradeNo := genTokenPoolSubscriptionTradeNo()
	desc := fmt.Sprintf("Pool subscription: %s", displayPoolName)

	timeExpire := time.Now().Add(time.Duration(model.TokenPoolSubscriptionPendingReuseSeconds) * time.Second)
	codeURL, err := nativePrepayFunc(ctx, cfg, client, notifyURL, tradeNo, desc, amountFen, timeExpire)
	if err != nil {
		logger.LogError(c, "wechat native prepay failed: "+err.Error())
		common.ApiErrorMsg(c, "failed to create wechat pay order")
		return
	}

	closeSupersededNativePoolSubscriptionOrders(ctx, cfg, client, tokenId, pool.Id)

	order := &model.TokenPoolSubscriptionOrder{
		UserId:               userId,
		TokenId:              tokenId,
		PoolId:               pool.Id,
		AmountCny:            pool.MonthlyPriceCny,
		AmountTotalFen:       amountFen,
		Currency:             cur,
		BillingPeriodSeconds: period,
		PeriodMonths:         option.PeriodMonths,
		DiscountRatioBp:      option.DiscountRatioBp,
		IsUpgrade:            isUpgrade,
		UpgradedFromPoolId:   upgradeFromPoolId,
		CreditSecondsGranted: creditSeconds,
		TradeNo:              tradeNo,
		CodeUrl:              codeURL,
		PaymentType:          "native",
		Status:               common.TopUpStatusPending,
	}
	if err := model.InsertTokenPoolSubscriptionOrder(order); err != nil {
		logger.LogError(c, "insert token pool subscription order failed: "+err.Error())
		common.ApiErrorMsg(c, "failed to persist order")
		return
	}

	common.ApiSuccess(c, tokenPoolCheckoutPayload(order, displayPoolName, false))
}

func tokenPoolCheckoutPayload(order *model.TokenPoolSubscriptionOrder, poolName string, reused bool) gin.H {
	if order == nil {
		return gin.H{}
	}
	return gin.H{
		"code_url":   order.CodeUrl,
		"trade_no":   order.TradeNo,
		"amount_fen": order.AmountTotalFen,
		"currency":   order.Currency,
		"pool_name":  poolName,
		"status":     order.Status,
		"reused":     reused,
	}
}

// GetTokenPoolSubscriptionOrderSelf returns checkout order status for the authenticated token.
// GET /api/usage/token/pool/subscription/order?trade_no=...
func GetTokenPoolSubscriptionOrderSelf(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Query("trade_no"))
	if tradeNo == "" {
		common.ApiErrorMsg(c, "trade_no required")
		return
	}
	tokenId := c.GetInt("token_id")
	if tokenId <= 0 {
		common.ApiErrorMsg(c, "invalid token")
		return
	}

	order, err := model.GetTokenPoolSubscriptionOrderForToken(tradeNo, tokenId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "order not found")
			return
		}
		common.ApiError(c, err)
		return
	}

	reconciled, reconcileErr := service.ReconcileTokenPoolSubscriptionOrderFromWeChat(c.Request.Context(), order)
	if reconcileErr != nil {
		logger.LogError(c, "wechat order query failed trade_no="+tradeNo+": "+reconcileErr.Error())
	}

	if reconciled {
		order, err = model.GetTokenPoolSubscriptionOrderForToken(tradeNo, tokenId)
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
		"complete_time":          order.CompleteTime,
		"reconciled_from_wechat": reconciled,
	})
}

func closeSupersededNativePoolSubscriptionOrders(ctx context.Context, cfg *wechatpay.Config, client *core.Client, tokenId, poolId int) {
	if cfg == nil || client == nil || tokenId <= 0 || poolId <= 0 {
		return
	}
	pending, err := model.ListPendingNativeTokenPoolSubscriptionOrders(tokenId, poolId)
	if err != nil || len(pending) == 0 {
		return
	}
	for _, o := range pending {
		if o.TradeNo == "" {
			continue
		}
		if err := closeNativeOrderFunc(ctx, cfg, client, o.TradeNo); err != nil {
			logger.LogError(ctx, "wechat close superseded native order failed trade_no="+o.TradeNo+": "+err.Error())
			continue
		}
		_ = model.MarkTokenPoolSubscriptionOrderExpired(o.TradeNo)
	}
}

// WeChatPayPoolSubscriptionNotify handles WeChat Pay v3 payment notifications for pool subscriptions.
func WeChatPayPoolSubscriptionNotify(c *gin.Context) {
	ctx := context.Background()
	_, cfg, err := wechatpayClientFunc(ctx)
	if err != nil || cfg == nil {
		if err != nil {
			logger.LogError(c, "wechat pay notify: client not available: "+err.Error())
		} else {
			logger.LogError(c, "wechat pay notify: client not available: nil config")
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "FAIL", "message": "not configured"})
		return
	}

	_, tx, err := parsePaymentNotifyFunc(ctx, cfg, c.Request)
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

	if err := service.FulfillTokenPoolSubscriptionFromTransaction(tx); err != nil {
		if errors.Is(err, model.ErrPoolSubscriptionOrderUnfulfillable) {
			logger.LogError(c, "complete token pool subscription permanent failure trade_no="+outNo+" err="+err.Error())
			c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "成功"})
			return
		}
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

	type poolSubscriptionOrderAdminItem struct {
		*model.TokenPoolSubscriptionOrder
		TokenName string `json:"token_name,omitempty"`
		PoolName  string `json:"pool_name,omitempty"`
	}

	out := make([]poolSubscriptionOrderAdminItem, 0, len(items))
	tokenIDs := make(map[int]struct{})
	poolIDs := make(map[int]struct{})
	for _, item := range items {
		if item == nil {
			continue
		}
		out = append(out, poolSubscriptionOrderAdminItem{TokenPoolSubscriptionOrder: item})
		if item.TokenId > 0 {
			tokenIDs[item.TokenId] = struct{}{}
		}
		if item.PoolId > 0 {
			poolIDs[item.PoolId] = struct{}{}
		}
	}

	tokenNames := make(map[int]string)
	if len(tokenIDs) > 0 {
		ids := make([]int, 0, len(tokenIDs))
		for id := range tokenIDs {
			ids = append(ids, id)
		}
		var tokens []model.Token
		if err := model.DB.Where("id IN ?", ids).Select("id", "name").Find(&tokens).Error; err == nil {
			for i := range tokens {
				tokenNames[tokens[i].Id] = tokens[i].Name
			}
		}
	}

	poolNames := make(map[int]string)
	if len(poolIDs) > 0 {
		ids := make([]int, 0, len(poolIDs))
		for id := range poolIDs {
			ids = append(ids, id)
		}
		var pools []model.Pool
		if err := model.DB.Where("id IN ?", ids).Select("id", "name").Find(&pools).Error; err == nil {
			for i := range pools {
				poolNames[pools[i].Id] = pools[i].Name
			}
		}
	}

	for i := range out {
		if name, ok := tokenNames[out[i].TokenId]; ok {
			out[i].TokenName = name
		}
		if name, ok := poolNames[out[i].PoolId]; ok {
			out[i].PoolName = name
		}
	}

	common.ApiSuccess(c, gin.H{
		"items":     out,
		"total":     total,
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
	})
}

// poolPlanDTO describes a single pool tier that a user may buy or upgrade to.
type poolPlanDTO struct {
	PoolId          int                    `json:"pool_id"`
	PoolName        string                 `json:"pool_name"`
	Description     string                 `json:"description,omitempty"`
	PlanCode        string                 `json:"plan_code"`
	PlanGroup       string                 `json:"plan_group"`
	PlanTier        int                    `json:"plan_tier"`
	DisplayName     string                 `json:"display_name"`
	DisplayOrder    int                    `json:"display_order"`
	MonthlyPriceCny float64                `json:"monthly_price_cny"`
	Currency        string                 `json:"currency"`
	PeriodOptions   []poolPeriodOptionDTO  `json:"period_options"`
}

func buildPoolPlanDTO(pool *model.Pool, options []model.PoolPeriodOption) poolPlanDTO {
	if pool == nil {
		return poolPlanDTO{}
	}
	cur := pool.BillingCurrency
	if cur == "" {
		cur = "CNY"
	}
	return poolPlanDTO{
		PoolId:          pool.Id,
		PoolName:        poolDisplayName(pool),
		Description:     strings.TrimSpace(pool.Description),
		PlanCode:        pool.PlanCode,
		PlanGroup:       pool.PlanGroup,
		PlanTier:        pool.PlanTier,
		DisplayName:     strings.TrimSpace(pool.DisplayName),
		DisplayOrder:    pool.DisplayOrder,
		MonthlyPriceCny: pool.MonthlyPriceCny,
		Currency:        cur,
		PeriodOptions:   buildPeriodOptionDTOs(pool.MonthlyPriceCny, options),
	}
}

// GetTokenPoolPlans returns the current pool for the token plus any same-plan-group upgrade targets.
// GET /api/usage/token/pool/plans
func GetTokenPoolPlans(c *gin.Context) {
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

	currentOptions, err := getPoolPeriodOptions(resolved.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	currentDTO := buildPoolPlanDTO(resolved, currentOptions)

	// Upgrade targets: same plan_group, higher plan_tier, enabled.
	upgradeTargets := make([]poolPlanDTO, 0)
	if strings.TrimSpace(resolved.PlanGroup) != "" {
		peers, err := model.GetPoolPlanGroupPeers(resolved)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for _, peer := range peers {
			if peer == nil || peer.PlanTier <= resolved.PlanTier {
				continue
			}
			if !model.PoolRequiresPaidSubscription(peer) {
				continue
			}
			peerOptions, err := getPoolPeriodOptions(peer.Id)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			upgradeTargets = append(upgradeTargets, buildPoolPlanDTO(peer, peerOptions))
		}
	}

	common.ApiSuccess(c, gin.H{
		"current":         currentDTO,
		"upgrade_targets": upgradeTargets,
	})
}

// QuoteTokenPoolSubscription returns the computed amount and (optional) upgrade credit for
// (pool_id, period_months, upgrade_from_pool_id). No side effects, no WeChat call.
// POST /api/usage/token/pool/subscription/quote
func QuoteTokenPoolSubscription(c *gin.Context) {
	var req tokenPoolSubscriptionQuoteRequest
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

	target := resolved
	if req.PoolId != resolved.Id {
		t, err := model.GetPoolById(req.PoolId)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				common.ApiErrorMsg(c, "target pool not found")
				return
			}
			common.ApiError(c, err)
			return
		}
		if t == nil || t.Status != model.PoolStatusEnabled {
			common.ApiErrorMsg(c, "target pool is not available")
			return
		}
		target = t
	}

	options, err := getPoolPeriodOptions(target.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	periodMonths := 1
	if req.PeriodMonths != nil && *req.PeriodMonths > 0 {
		periodMonths = *req.PeriodMonths
	}
	option, ok := findPeriodOption(options, periodMonths)
	if !ok {
		common.ApiErrorMsg(c, "period_months not offered for this pool")
		return
	}
	baseAmountFen := computeAmountFen(target.MonthlyPriceCny, option.PeriodMonths, 10000)
	amountFen := computeAmountFen(target.MonthlyPriceCny, option.PeriodMonths, option.DiscountRatioBp)

	payload := gin.H{
		"pool_id":              target.Id,
		"pool_name":            poolDisplayName(target),
		"period_months":        option.PeriodMonths,
		"discount_ratio_bp":    option.DiscountRatioBp,
		"base_amount_fen":      baseAmountFen,
		"amount_fen":           amountFen,
		"currency":             target.BillingCurrency,
		"period_options":       buildPeriodOptionDTOs(target.MonthlyPriceCny, options),
		"is_upgrade":           false,
		"upgraded_from_pool_id": 0,
		"credit_seconds":       int64(0),
		"credit_days":          0,
	}

	// Upgrade credit calculation.
	if req.UpgradeFromPoolId != nil && *req.UpgradeFromPoolId > 0 && *req.UpgradeFromPoolId != target.Id {
		fromPoolId := *req.UpgradeFromPoolId
		fromPool, err := model.GetPoolById(fromPoolId)
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
		if strings.TrimSpace(fromPool.PlanGroup) == "" || fromPool.PlanGroup != target.PlanGroup {
			common.ApiErrorMsg(c, "upgrade_from_pool_id must share the same plan_group as target")
			return
		}
		if fromPool.PlanTier >= target.PlanTier {
			common.ApiErrorMsg(c, "upgrade_from_pool_id must be a lower tier than target")
			return
		}
		fromSub, err := model.GetTokenPoolSubscription(tokenId, fromPoolId)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiError(c, err)
			return
		}
		nowTs := common.GetTimestamp()
		var creditSeconds int64
		if fromSub != nil && fromSub.PeriodEnd > nowTs {
			creditSeconds = computeUpgradeCreditSeconds(fromSub.PeriodEnd-nowTs, fromPool.MonthlyPriceCny, target.MonthlyPriceCny)
		}
		creditDays := int(creditSeconds / 86400)
		payload["is_upgrade"] = true
		payload["upgraded_from_pool_id"] = fromPoolId
		payload["credit_seconds"] = creditSeconds
		payload["credit_days"] = creditDays
	}

	common.ApiSuccess(c, payload)
}
