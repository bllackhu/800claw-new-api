package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/capability_setting"

	"github.com/gin-gonic/gin"
)

// CapabilityAuth 能力授权中间件。
//
// 只对能力注册表（constant.GetCapabilityByPath）映射的端点生效，普通 LLM 路径直接放行，聊天调用零影响。
// 行为：
//   - 该能力 enforcement 未开启 → 直接放行（兼容存量，走原有计费）
//   - Token 无该能力授权 → 403"该能力未开通"
//   - count 模式 remaining <= 0 → 403"次数用尽"
//   - wallet 模式 → 校验钱包余额（预扣阶段再正式扣款）
//
// 通过后写上下文：capability / capability_mode / capability_unit / capability_price，供计费阶段使用。
func CapabilityAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		capability := constant.GetCapabilityByPath(c.Request.URL.Path)
		if capability == "" {
			c.Next()
			return
		}
		if !capability_setting.IsCapabilityEnabled(capability) {
			c.Next()
			return
		}

		tokenId := c.GetInt("token_id")
		if tokenId <= 0 {
			abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgCapabilityNotGranted))
			return
		}

		grantMap, err := model.GetTokenCapabilityMap(tokenId)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, i18n.T(c, i18n.MsgCapabilityLoadFailed))
			return
		}
		grant, ok := grantMap[capability]
		if !ok {
			abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgCapabilityNotGranted))
			return
		}

		switch grant.Mode {
		case constant.CapabilityModeCount:
			if grant.Remaining <= 0 {
				abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgCapabilityExhausted))
				return
			}
		case constant.CapabilityModeWallet:
			if !common.GetContextKeyBool(c, constant.ContextKeyTokenUnlimited) {
				if c.GetInt("token_quota") <= 0 {
					abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgCapabilityInsufficientQuota))
					return
				}
			}
		}

		common.SetContextKey(c, constant.ContextKeyCapability, capability)
		common.SetContextKey(c, constant.ContextKeyCapabilityMode, grant.Mode)
		common.SetContextKey(c, constant.ContextKeyCapabilityUnit, constant.GetCapabilityUnit(capability))
		c.Set(string(constant.ContextKeyCapabilityPrice), capability_setting.GetCapabilityPrice(capability))
		c.Next()
	}
}
