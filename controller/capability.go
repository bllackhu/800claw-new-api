package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/capability_setting"

	"github.com/gin-gonic/gin"
)

// CapabilitySettingsResponse 能力设置 + 聚合统计（管理端看板）
type CapabilitySettingsResponse struct {
	Capabilities  map[string]capability_setting.CapabilityConfig `json:"capabilities"`
	ConsumedTotal map[string]int64                               `json:"consumed_total"`
}

func GetCapabilitySettings(c *gin.Context) {
	stats, err := model.GetCapabilityStatsMap()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, CapabilitySettingsResponse{
		Capabilities:  capability_setting.GetCapabilitiesCopy(),
		ConsumedTotal: stats,
	})
}

func UpdateCapabilitySettings(c *gin.Context) {
	var req struct {
		Capabilities map[string]capability_setting.CapabilityConfig `json:"capabilities"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := capability_setting.UpdateCapabilities(req.Capabilities); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetCapabilityStats(c *gin.Context) {
	stats, err := model.GetCapabilityStatsMap()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}
