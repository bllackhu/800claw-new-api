package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type tokenPoolSubscriptionAdminItem struct {
	model.TokenPoolSubscription
	Active    bool   `json:"active"`
	TokenName string `json:"token_name,omitempty"`
	PoolName  string `json:"pool_name,omitempty"`
}

type putTokenPoolSubscriptionRequest struct {
	TokenId     int    `json:"token_id"`
	PoolId      int    `json:"pool_id"`
	PeriodEnd   int64  `json:"period_end"`
	PeriodStart *int64 `json:"period_start"`
}

// GetTokenPoolSubscriptions lists token pool subscription rows (admin).
// GET /api/pool/token_subscriptions?token_id=&pool_id=
func GetTokenPoolSubscriptions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	tokenId, _ := strconv.Atoi(c.Query("token_id"))
	poolId, _ := strconv.Atoi(c.Query("pool_id"))

	items, total, err := model.ListTokenPoolSubscriptions(
		pageInfo.GetStartIdx(),
		pageInfo.GetPageSize(),
		tokenId,
		poolId,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	now := common.GetTimestamp()
	out := make([]tokenPoolSubscriptionAdminItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		out = append(out, tokenPoolSubscriptionAdminItem{
			TokenPoolSubscription: *item,
			Active:                item.PeriodEnd >= now,
		})
	}

	enrichTokenPoolSubscriptionItems(out)

	common.ApiSuccess(c, gin.H{
		"items":     out,
		"total":     total,
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
	})
}

// PutTokenPoolSubscription upserts subscription period_end for (token_id, pool_id) (admin).
// PUT /api/pool/token_subscription
func PutTokenPoolSubscription(c *gin.Context) {
	var req putTokenPoolSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.TokenId <= 0 || req.PoolId <= 0 {
		common.ApiErrorMsg(c, "invalid token_id or pool_id")
		return
	}
	if req.PeriodEnd <= 0 {
		common.ApiErrorMsg(c, "invalid period_end")
		return
	}

	periodStart := int64(0)
	if req.PeriodStart != nil {
		periodStart = *req.PeriodStart
	}

	sub, err := model.AdminUpsertTokenPoolSubscription(req.TokenId, req.PoolId, periodStart, req.PeriodEnd)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	logger.LogInfo(c, "admin token pool subscription upsert token_id="+strconv.Itoa(req.TokenId)+
		" pool_id="+strconv.Itoa(req.PoolId)+" period_end="+strconv.FormatInt(req.PeriodEnd, 10))

	now := common.GetTimestamp()
	item := tokenPoolSubscriptionAdminItem{
		TokenPoolSubscription: *sub,
		Active:                sub.PeriodEnd >= now,
	}
	enrichTokenPoolSubscriptionItems([]tokenPoolSubscriptionAdminItem{item})
	common.ApiSuccess(c, item)
}

// AdminReconcileWechatOrder triggers WeChat order status query for a given trade_no (admin only).
// POST /api/pool/subscription_orders/:trade_no/reconcile
func AdminReconcileWechatOrder(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Param("trade_no"))
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

	if order.Status != common.TopUpStatusPending {
		common.ApiSuccess(c, gin.H{
			"order":      order,
			"reconciled": false,
			"message":    "order is not pending, no reconcile needed",
		})
		return
	}

	reconciled, reconcileErr := reconcileTokenPoolSubscriptionOrderFromWeChat(c.Request.Context(), order)
	if reconcileErr != nil {
		logger.LogError(c, "admin reconcile failed trade_no="+tradeNo+": "+reconcileErr.Error())
		common.ApiErrorMsg(c, "reconcile failed: "+reconcileErr.Error())
		return
	}

	if reconciled {
		order, err = model.GetTokenPoolSubscriptionOrderByTradeNo(tradeNo)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}

	common.ApiSuccess(c, gin.H{
		"order":      order,
		"reconciled": reconciled,
	})
}

func enrichTokenPoolSubscriptionItems(items []tokenPoolSubscriptionAdminItem) {
	if len(items) == 0 {
		return
	}
	tokenIDs := make(map[int]struct{})
	poolIDs := make(map[int]struct{})
	for i := range items {
		if items[i].TokenId > 0 {
			tokenIDs[items[i].TokenId] = struct{}{}
		}
		if items[i].PoolId > 0 {
			poolIDs[items[i].PoolId] = struct{}{}
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

	for i := range items {
		if name, ok := tokenNames[items[i].TokenId]; ok {
			items[i].TokenName = name
		}
		if name, ok := poolNames[items[i].PoolId]; ok {
			items[i].PoolName = name
		}
	}
}
