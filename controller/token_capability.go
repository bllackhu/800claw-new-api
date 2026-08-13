package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type TokenCapabilityRequest struct {
	Capability string `json:"capability"`
	Mode       string `json:"mode"`
	Granted    int64  `json:"granted"`
}

// verifyTokenOwnership 校验 token 归属当前用户，返回 tokenId（<=0 表示失败已处理）
func verifyTokenOwnership(c *gin.Context) int {
	userId := c.GetInt("id")
	tokenId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return 0
	}
	if _, err := model.GetTokenByIds(tokenId, userId); err != nil {
		common.ApiError(c, err)
		return 0
	}
	return tokenId
}

func GetTokenCapabilities(c *gin.Context) {
	tokenId := verifyTokenOwnership(c)
	if tokenId <= 0 {
		return
	}
	capabilities, err := model.GetTokenCapabilities(tokenId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 注册表：供前端选择可授权的能力
	registry := make([]map[string]any, 0, len(constant.CapabilityRegistry))
	for _, meta := range constant.CapabilityRegistry {
		registry = append(registry, map[string]any{
			"name": meta.Name,
			"unit": meta.UnitType,
		})
	}
	common.ApiSuccess(c, gin.H{
		"capabilities": capabilities,
		"registry":     registry,
	})
}

func AddOrUpdateTokenCapability(c *gin.Context) {
	tokenId := verifyTokenOwnership(c)
	if tokenId <= 0 {
		return
	}
	var req TokenCapabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if !constant.IsValidCapability(req.Capability) {
		common.ApiErrorI18n(c, i18n.MsgCapabilityInvalidName)
		return
	}
	if req.Mode != constant.CapabilityModeCount && req.Mode != constant.CapabilityModeWallet {
		common.ApiErrorI18n(c, i18n.MsgCapabilityInvalidMode)
		return
	}
	if req.Granted < 0 {
		common.ApiErrorI18n(c, i18n.MsgCapabilityInvalidGranted)
		return
	}

	tc := &model.TokenCapability{
		TokenId:    tokenId,
		Capability: req.Capability,
		Mode:       req.Mode,
		Granted:    req.Granted,
	}
	if req.Mode == constant.CapabilityModeCount {
		tc.Remaining = req.Granted
	} else {
		tc.Remaining = 0
	}
	if err := tc.Upsert(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, tc)
}

func DeleteTokenCapability(c *gin.Context) {
	tokenId := verifyTokenOwnership(c)
	if tokenId <= 0 {
		return
	}
	capability := c.Param("capability")
	if !constant.IsValidCapability(capability) {
		common.ApiErrorI18n(c, i18n.MsgCapabilityInvalidName)
		return
	}
	// 通过 token_id + capability 定位并删除
	if err := model.DeleteTokenCapabilityByTokenAndCapability(tokenId, capability); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
