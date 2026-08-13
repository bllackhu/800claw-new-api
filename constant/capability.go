package constant

import "strings"

// Capability（能力）类型定义。
// 能力 = 独立端点的通用第三方能力接口（语音识别、搜索、图片生成等）。
// 通过 Token 级授权 + 独立计量进行管控，与渠道/路由解耦。
// 新增能力：在 CapabilityRegistry 增加一条元信息即可。

const (
	CapabilitySpeechRecognition = "speech_recognition"
)

// 计量单位类型
const (
	CapabilityUnitCount   = "count"   // 次
	CapabilityUnitMinutes = "minutes" // 分钟
	CapabilityUnitSeconds = "seconds" // 秒
	CapabilityUnitImages  = "images"  // 张
)

// Token 能力授权模式
const (
	CapabilityModeCount  = "count"  // 次数包：不消耗钱包 quota，扣 remaining
	CapabilityModeWallet = "wallet" // 按次计费：从钱包 quota 按零售价扣费
)

// CapabilityMeta 描述一个能力的元信息
type CapabilityMeta struct {
	Name         string   // 能力名（唯一标识）
	UnitType     string   // 计量单位
	DefaultPrice float64  // 默认零售价（USD / 单位）
	Paths        []string // 该能力对应的端点路径前缀
}

var CapabilityRegistry = []CapabilityMeta{
	{
		Name:         CapabilitySpeechRecognition,
		UnitType:     CapabilityUnitMinutes,
		DefaultPrice: 0.006, // 参考 whisper-1 $0.006 / 分钟
		Paths:        []string{"/v1/audio/transcriptions"},
	},
}

// GetCapabilityMeta 返回能力元信息，不存在返回 nil
func GetCapabilityMeta(name string) *CapabilityMeta {
	for i := range CapabilityRegistry {
		if CapabilityRegistry[i].Name == name {
			return &CapabilityRegistry[i]
		}
	}
	return nil
}

// IsValidCapability 判断能力名是否已注册
func IsValidCapability(name string) bool {
	return GetCapabilityMeta(name) != nil
}

// GetCapabilityByPath 通过请求路径前缀找到对应能力，不存在返回空字符串
func GetCapabilityByPath(path string) string {
	for i := range CapabilityRegistry {
		for _, p := range CapabilityRegistry[i].Paths {
			if strings.HasPrefix(path, p) {
				return CapabilityRegistry[i].Name
			}
		}
	}
	return ""
}

// GetCapabilityUnit 返回能力的计量单位，未知能力默认按"次"
func GetCapabilityUnit(name string) string {
	meta := GetCapabilityMeta(name)
	if meta == nil {
		return CapabilityUnitCount
	}
	return meta.UnitType
}
