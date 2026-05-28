package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type tokenPoolSubscriptionAdminItem struct {
	model.TokenPoolSubscription
	Active bool `json:"active"`
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
	common.ApiSuccess(c, tokenPoolSubscriptionAdminItem{
		TokenPoolSubscription: *sub,
		Active:                sub.PeriodEnd >= now,
	})
}
