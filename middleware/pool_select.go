package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func PoolSelect() func(c *gin.Context) {
	return func(c *gin.Context) {
		if !common.PoolEnabled {
			c.Next()
			return
		}

		userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
		tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
		usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		pool, err := model.ResolvePoolForContext(userId, tokenId, usingGroup)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "no available pool for current user/group")
				return
			}
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "failed to resolve pool")
			return
		}
		if pool == nil {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "no available pool for current user/group")
			return
		}

		common.SetContextKey(c, constant.ContextKeyPoolId, pool.Id)
		common.SetContextKey(c, constant.ContextKeyPoolName, pool.Name)
		common.SetContextKey(c, constant.ContextKeyPoolScopeKey, "user:"+strconv.Itoa(userId))
		requireSub := common.GetContextKeyBool(c, constant.ContextKeyTokenRequirePoolSubscription)
		if model.TokenRelayRequiresPoolSubscriptionCheck(pool, requireSub) {
			if tokenId <= 0 {
				abortWithOpenAiMessage(c, http.StatusPaymentRequired, "this pool requires an API token with an active paid subscription")
				return
			}
			ok, err := model.TokenHasActivePoolSubscription(tokenId, pool.Id)
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "failed to verify pool subscription")
				return
			}
			if !ok {
				// Lazy reconcile: if user paid via Native QR but notify was lost, query WeChat once.
				if _, reconcileErr := service.MaybeReconcilePendingPoolSubscription(c.Request.Context(), tokenId, pool.Id); reconcileErr != nil {
					logger.LogError(c.Request.Context(), "pool subscription lazy reconcile failed: "+reconcileErr.Error())
				} else {
					ok, err = model.TokenHasActivePoolSubscription(tokenId, pool.Id)
					if err != nil {
						abortWithOpenAiMessage(c, http.StatusInternalServerError, "failed to verify pool subscription")
						return
					}
				}
			}
			if !ok {
				// First-request free window: if no subscription row exists for this (token, pool),
				// auto-grant a one-time trial anchored on the current request (period_end at
				// 23:59:59 Asia/Shanghai on now + pool.BillingPeriodSeconds days). Tokens whose
				// trial has already been consumed (row exists but expired) still get 402 here.
				granted, err := model.GrantFirstRequestTrialIfEligible(tokenId, pool.Id, pool.BillingPeriodSeconds)
				if err != nil {
					abortWithOpenAiMessage(c, http.StatusInternalServerError, "failed to verify pool subscription")
					return
				}
				if !granted {
					abortWithOpenAiMessage(c, http.StatusPaymentRequired, "active pool subscription required for this token")
					return
				}
			}
		}
		c.Next()
	}
}
