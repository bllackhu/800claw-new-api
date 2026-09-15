package capability_setting

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

// CapabilityConfig 单个能力的运营配置
type CapabilityConfig struct {
	PricePerUnit   float64 `json:"price_per_unit"`  // 零售价（USD / 单位）
	PurchasedTotal int64   `json:"purchased_total"` // 订阅包采购总量（0 表示不限 / 纯按次计费）
	Enabled        bool    `json:"enabled"`         // enforcement 开关（默认关，开启后未授权 Token 调用该能力返回 403）
}

// CapabilitySetting 能力运营配置（通过 config.GlobalConfig 持久化）
type CapabilitySetting struct {
	Capabilities *types.RWMap[string, CapabilityConfig] `json:"capabilities"`
}

var capabilitySetting CapabilitySetting

func init() {
	capabilitySetting = CapabilitySetting{Capabilities: types.NewRWMap[string, CapabilityConfig]()}
	ensureRegisteredCapabilities()
	config.GlobalConfig.Register("capability_setting", &capabilitySetting)
}

// ensureRegisteredCapabilities 确保所有已注册的能力都存在，缺失时用注册表默认值补上
func ensureRegisteredCapabilities() {
	for _, meta := range constant.CapabilityRegistry {
		if _, ok := capabilitySetting.Capabilities.Get(meta.Name); !ok {
			capabilitySetting.Capabilities.Set(meta.Name, CapabilityConfig{
				PricePerUnit: meta.DefaultPrice,
				Enabled:      false,
			})
		}
	}
}

// ReseedFromRegistry 重新补齐所有已注册的能力（数据库加载后调用，防止缺失/空数据清空注册表）
func ReseedFromRegistry() {
	ensureRegisteredCapabilities()
}

// GetCapabilitySetting 返回能力设置（用于管理端编辑）
func GetCapabilitySetting() *CapabilitySetting {
	return &capabilitySetting
}

// IsCapabilityEnabled 判断该能力的 enforcement 开关是否开启
func IsCapabilityEnabled(name string) bool {
	cfg, ok := capabilitySetting.Capabilities.Get(name)
	if !ok {
		return false
	}
	return cfg.Enabled
}

// GetCapabilityPrice 返回能力的零售价（USD / 单位）
func GetCapabilityPrice(name string) float64 {
	cfg, ok := capabilitySetting.Capabilities.Get(name)
	if !ok {
		return 0
	}
	return cfg.PricePerUnit
}

// GetPurchasedTotal 返回能力订阅包采购总量
func GetPurchasedTotal(name string) int64 {
	cfg, ok := capabilitySetting.Capabilities.Get(name)
	if !ok {
		return 0
	}
	return cfg.PurchasedTotal
}

// GetCapabilitiesCopy 返回全部能力配置副本（用于管理端展示）
func GetCapabilitiesCopy() map[string]CapabilityConfig {
	return capabilitySetting.Capabilities.ReadAll()
}

// UpdateCapabilities 批量更新能力配置，仅接受已注册的能力。
// 采用合并语义：只更新提交中的能力，缺失的注册能力保留原值，避免误清空。
func UpdateCapabilities(configs map[string]CapabilityConfig) error {
	for name, cfg := range configs {
		if !constant.IsValidCapability(name) {
			continue
		}
		capabilitySetting.Capabilities.Set(name, cfg)
	}
	ensureRegisteredCapabilities()
	return nil
}

// MarshalCapabilitiesJSON 返回能力配置的 JSON 字符串（用于持久化到 options 表）
func MarshalCapabilitiesJSON() string {
	return capabilitySetting.Capabilities.MarshalJSONString()
}
