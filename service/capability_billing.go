package service

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const capabilityPreConsumedUnitsKey = "capability_preconsumed_units"

// IsCapabilityRequest 判断当前请求是否处于能力计费模式（由 CapabilityAuth 写入上下文）
func IsCapabilityRequest(c *gin.Context) bool {
	return common.GetContextKeyString(c, constant.ContextKeyCapability) != ""
}

func capabilityGroupRatio(relayInfo *relaycommon.RelayInfo) float64 {
	return ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
}

// capabilityQuotaFromUnits 单位数 → quota 单位（价 × 单位 × 分组倍率 × QuotaPerUnit）
func capabilityQuotaFromUnits(relayInfo *relaycommon.RelayInfo, price float64, units int64) int {
	quota := int(price * float64(units) * capabilityGroupRatio(relayInfo) * common.QuotaPerUnit)
	if quota < 1 {
		quota = 1
	}
	return quota
}

// CapabilityUnitsFromDuration 按能力计量单位将时长（毫秒）向上取整换算为整数单位。
// durationMs <= 0 时返回 1（最小计量单位，保证 count 模式至少扣 1）。
func CapabilityUnitsFromDuration(c *gin.Context, durationMs int64) int64 {
	unit := common.GetContextKeyString(c, constant.ContextKeyCapabilityUnit)
	if durationMs <= 0 {
		return 1
	}
	div := int64(1)
	switch unit {
	case constant.CapabilityUnitMinutes:
		div = 60000
	case constant.CapabilityUnitSeconds:
		div = 1000
	default:
		return 1
	}
	units := durationMs / div
	if durationMs%div != 0 {
		units++
	}
	if units < 1 {
		units = 1
	}
	return units
}

// PreConsumeCapabilityBilling 能力计费预扣，失败返回非 nil 需中断请求。
//   - count 模式：预扣 1 估算单位（原子扣减，remaining 不足则失败）
//   - wallet 模式：复用 BillingSession 预扣（按零售价 × 估算 1 单位 × groupRatio）
func PreConsumeCapabilityBilling(c *gin.Context, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	capability := common.GetContextKeyString(c, constant.ContextKeyCapability)
	if capability == "" {
		return nil
	}
	mode := common.GetContextKeyString(c, constant.ContextKeyCapabilityMode)
	switch mode {
	case constant.CapabilityModeCount:
		estimate := int64(1)
		ok, err := model.DecreaseTokenCapability(relayInfo.TokenId, relayInfo.TokenKey, capability, estimate)
		if err != nil {
			return types.NewError(err, types.ErrorCodeCapabilityPreConsumeFailed)
		}
		if !ok {
			return types.NewError(errors.New("capability count exhausted"), types.ErrorCodeCapabilityExhausted, types.ErrOptionWithStatusCode(http.StatusForbidden))
		}
		c.Set(capabilityPreConsumedUnitsKey, estimate)
		return nil
	case constant.CapabilityModeWallet:
		price := c.GetFloat64(string(constant.ContextKeyCapabilityPrice))
		preConsumedQuota := capabilityQuotaFromUnits(relayInfo, price, 1)
		return PreConsumeBilling(c, preConsumedQuota, relayInfo)
	}
	return nil
}

// RefundCapabilityBilling 能力请求失败时的回退
func RefundCapabilityBilling(c *gin.Context, relayInfo *relaycommon.RelayInfo) {
	capability := common.GetContextKeyString(c, constant.ContextKeyCapability)
	if capability == "" {
		return
	}
	mode := common.GetContextKeyString(c, constant.ContextKeyCapabilityMode)
	if mode == constant.CapabilityModeCount {
		if v, ok := c.Get(capabilityPreConsumedUnitsKey); ok {
			if pre, ok := v.(int64); ok && pre > 0 {
				if err := model.IncreaseTokenCapability(relayInfo.TokenId, relayInfo.TokenKey, capability, pre); err != nil {
					logger.LogError(c, "failed to refund capability count: "+err.Error())
				}
			}
		}
		return
	}
	if relayInfo.Billing != nil {
		relayInfo.Billing.Refund(c)
	}
}

// PostCapabilityConsume 能力请求成功后的结算。actualUnits 为实际用量（已向上取整）。
// durationMs 为可选时长（毫秒），用于日志与统计。
func PostCapabilityConsume(c *gin.Context, relayInfo *relaycommon.RelayInfo, actualUnits int64, durationMs int64) {
	capability := common.GetContextKeyString(c, constant.ContextKeyCapability)
	if capability == "" {
		return
	}
	mode := common.GetContextKeyString(c, constant.ContextKeyCapabilityMode)
	price := c.GetFloat64(string(constant.ContextKeyCapabilityPrice))
	var quota int

	switch mode {
	case constant.CapabilityModeCount:
		preUnits := int64(1)
		if v, ok := c.Get(capabilityPreConsumedUnitsKey); ok {
			if p, ok := v.(int64); ok && p > 0 {
				preUnits = p
			}
		}
		delta := actualUnits - preUnits
		if delta > 0 {
			if ok, err := model.DecreaseTokenCapability(relayInfo.TokenId, relayInfo.TokenKey, capability, delta); err != nil {
				logger.LogError(c, "failed to settle extra capability units: "+err.Error())
			} else if !ok {
				logger.LogWarn(c, fmt.Sprintf("capability %s remaining insufficient to settle extra %d units", capability, delta))
			}
		} else if delta < 0 {
			if err := model.IncreaseTokenCapability(relayInfo.TokenId, relayInfo.TokenKey, capability, -delta); err != nil {
				logger.LogError(c, "failed to refund capability units: "+err.Error())
			}
		}
		quota = 0
	case constant.CapabilityModeWallet:
		quota = capabilityQuotaFromUnits(relayInfo, price, actualUnits)
		if err := SettleBilling(c, relayInfo, quota); err != nil {
			logger.LogError(c, "failed to settle capability billing: "+err.Error())
		}
	}

	model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
	model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)

	if err := model.IncreaseCapabilityConsumedTotal(capability, actualUnits); err != nil {
		logger.LogError(c, "failed to increase capability consumed total: "+err.Error())
	}

	recordCapabilityConsumeLog(c, relayInfo, capability, mode, actualUnits, durationMs, price, quota)
}

func recordCapabilityConsumeLog(c *gin.Context, relayInfo *relaycommon.RelayInfo, capability, mode string, units int64, durationMs int64, price float64, quota int) {
	other := map[string]interface{}{
		"capability":        capability,
		"capability_mode":   mode,
		"capability_unit":   constant.GetCapabilityUnit(capability),
		"capability_units":  units,
		"capability_price":  price,
		"capability_quota":  quota,
	}
	if durationMs > 0 {
		other["duration_ms"] = durationMs
	}
	// PromptTokens 记实际用量（STT 记秒数），便于日志与统计
	promptTokens := 0
	if durationMs > 0 {
		promptTokens = int(durationMs / 1000)
	}
	model.RecordConsumeLog(c, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     promptTokens,
		ModelName:        capability,
		TokenName:        c.GetString("token_name"),
		Quota:            quota,
		Content:          fmt.Sprintf("能力调用：%s（%s 模式），用量 %d %s", capability, mode, units, constant.GetCapabilityUnit(capability)),
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(time.Now().Unix() - relayInfo.StartTime.Unix()),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
	})
}
