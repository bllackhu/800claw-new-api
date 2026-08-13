package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

// TokenCapability Token 的能力授权与计数。
// mode = count（次数包，扣 remaining）/ wallet（按次从钱包扣费，不使用 remaining）
type TokenCapability struct {
	Id         int    `json:"id"`
	TokenId    int    `json:"token_id" gorm:"index"`
	Capability string `json:"capability" gorm:"type:varchar(64)"`
	Mode       string `json:"mode" gorm:"type:varchar(16);default:count"`
	Granted    int64  `json:"granted" gorm:"bigint;default:0"`  // count 模式：初始授权次数
	Remaining  int64  `json:"remaining" gorm:"bigint;default:0"` // count 模式：剩余次数
	CreatedAt  int64  `json:"created_time" gorm:"bigint"`
	UpdatedAt  int64  `json:"updated_time" gorm:"bigint"`
}

func GetTokenCapabilities(tokenId int) ([]*TokenCapability, error) {
	var capabilities []*TokenCapability
	err := DB.Where("token_id = ?", tokenId).Find(&capabilities).Error
	return capabilities, err
}

func GetTokenCapability(tokenId int, capability string) (*TokenCapability, error) {
	var tc TokenCapability
	err := DB.Where("token_id = ? AND capability = ?", tokenId, capability).First(&tc).Error
	if err != nil {
		return nil, err
	}
	return &tc, nil
}

func (tc *TokenCapability) Insert() error {
	tc.CreatedAt = common.GetTimestamp()
	tc.UpdatedAt = tc.CreatedAt
	err := DB.Create(tc).Error
	if err == nil {
		asyncInvalidateCapabilityCache(tc.TokenId)
	}
	return err
}

func (tc *TokenCapability) Update() error {
	tc.UpdatedAt = common.GetTimestamp()
	err := DB.Model(&TokenCapability{}).Where("id = ?", tc.Id).Updates(map[string]interface{}{
		"mode":       tc.Mode,
		"granted":    tc.Granted,
		"remaining":  tc.Remaining,
		"updated_at": tc.UpdatedAt,
	}).Error
	if err == nil {
		asyncInvalidateCapabilityCache(tc.TokenId)
	}
	return err
}

// Upsert 幂等新增/更新授权（同 token 同 capability 唯一）
func (tc *TokenCapability) Upsert() error {
	now := common.GetTimestamp()
	tc.UpdatedAt = now
	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing TokenCapability
		err := tx.Where("token_id = ? AND capability = ?", tc.TokenId, tc.Capability).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			tc.CreatedAt = now
			return tx.Create(tc).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&existing).Updates(map[string]interface{}{
			"mode":       tc.Mode,
			"granted":    tc.Granted,
			"remaining":  tc.Remaining,
			"updated_at": now,
		}).Error
	})
	if err == nil {
		asyncInvalidateCapabilityCache(tc.TokenId)
	}
	return err
}

func DeleteTokenCapability(id int, tokenId int) error {
	err := DB.Where("id = ? AND token_id = ?", id, tokenId).Delete(&TokenCapability{}).Error
	if err == nil {
		asyncInvalidateCapabilityCache(tokenId)
	}
	return err
}

// DeleteTokenCapabilityByTokenAndCapability 按 token + 能力名删除授权
func DeleteTokenCapabilityByTokenAndCapability(tokenId int, capability string) error {
	err := DB.Where("token_id = ? AND capability = ?", tokenId, capability).Delete(&TokenCapability{}).Error
	if err == nil {
		asyncInvalidateCapabilityCache(tokenId)
	}
	return err
}

// DecreaseTokenCapability 原子扣减 remaining，防止超扣（remaining >= amount 才成功）
// 返回 (success, err)
func DecreaseTokenCapability(tokenId int, tokenKey string, capability string, amount int64) (bool, error) {
	if amount <= 0 {
		return false, errors.New("amount 必须为正数")
	}
	res := DB.Model(&TokenCapability{}).
		Where("token_id = ? AND capability = ? AND remaining >= ?", tokenId, capability, amount).
		Updates(map[string]interface{}{
			"remaining":  gorm.Expr("remaining - ?", amount),
			"updated_at": common.GetTimestamp(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, nil
	}
	asyncInvalidateCapabilityCache(tokenId)
	return true, nil
}

// IncreaseTokenCapability 回退 remaining（失败回滚用）
func IncreaseTokenCapability(tokenId int, tokenKey string, capability string, amount int64) error {
	if amount <= 0 {
		return errors.New("amount 必须为正数")
	}
	err := DB.Model(&TokenCapability{}).
		Where("token_id = ? AND capability = ?", tokenId, capability).
		Updates(map[string]interface{}{
			"remaining":  gorm.Expr("remaining + ?", amount),
			"updated_at": common.GetTimestamp(),
		}).Error
	if err == nil {
		asyncInvalidateCapabilityCache(tokenId)
	}
	return err
}

func asyncInvalidateCapabilityCache(tokenId int) {
	if !common.RedisEnabled {
		return
	}
	gopool.Go(func() {
		err := cacheDeleteTokenCapabilities(tokenId)
		if err != nil {
			common.SysLog("failed to delete capability cache: " + err.Error())
		}
	})
}

// GetTokenCapabilityMap 返回 token 的能力授权映射（capability -> 授权）。
// 优先读 Redis 缓存，未命中则回源 DB 并回填缓存。
func GetTokenCapabilityMap(tokenId int) (map[string]*TokenCapability, error) {
	if common.RedisEnabled {
		if cached, err := cacheGetTokenCapabilities(tokenId); err == nil {
			return cached, nil
		}
	}
	capabilities, err := GetTokenCapabilities(tokenId)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*TokenCapability, len(capabilities))
	for _, tc := range capabilities {
		m[tc.Capability] = tc
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			err := cacheSetTokenCapabilities(tokenId, capabilities)
			if err != nil {
				common.SysLog("failed to cache capabilities: " + err.Error())
			}
		})
	}
	return m, nil
}

// ---- 能力聚合统计（订阅包剩余量管理）----

type CapabilityStats struct {
	Capability    string `json:"capability" gorm:"primaryKey;type:varchar(64)"`
	ConsumedTotal int64  `json:"consumed_total" gorm:"bigint;default:0"`
	UpdatedAt     int64  `json:"updated_time" gorm:"bigint"`
}

// IncreaseCapabilityConsumedTotal 累加能力全局消耗量
func IncreaseCapabilityConsumedTotal(capability string, amount int64) error {
	if amount <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var stats CapabilityStats
		err := tx.Where("capability = ?", capability).First(&stats).Error
		if err == gorm.ErrRecordNotFound {
			return tx.Create(&CapabilityStats{
				Capability:    capability,
				ConsumedTotal: amount,
				UpdatedAt:     common.GetTimestamp(),
			}).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&stats).Updates(map[string]interface{}{
			"consumed_total": gorm.Expr("consumed_total + ?", amount),
			"updated_at":     common.GetTimestamp(),
		}).Error
	})
}

// GetCapabilityStatsMap 返回全部能力聚合消耗
func GetCapabilityStatsMap() (map[string]int64, error) {
	var stats []*CapabilityStats
	err := DB.Find(&stats).Error
	if err != nil {
		return nil, err
	}
	m := make(map[string]int64, len(stats))
	for _, s := range stats {
		m[s.Capability] = s.ConsumedTotal
	}
	return m, nil
}

// ---- Redis 缓存 ----

func capabilityCacheKey(tokenId int) string {
	return fmt.Sprintf("capability:token:%d", tokenId)
}

func cacheSetTokenCapabilities(tokenId int, capabilities []*TokenCapability) error {
	if len(capabilities) == 0 {
		return nil
	}
	data, err := common.Marshal(capabilities)
	if err != nil {
		return err
	}
	return common.RedisSet(capabilityCacheKey(tokenId), string(data), time.Duration(common.RedisKeyCacheSeconds())*time.Second)
}

func cacheGetTokenCapabilities(tokenId int) (map[string]*TokenCapability, error) {
	data, err := common.RedisGet(capabilityCacheKey(tokenId))
	if err != nil {
		return nil, err
	}
	var capabilities []*TokenCapability
	if err := common.Unmarshal([]byte(data), &capabilities); err != nil {
		return nil, err
	}
	m := make(map[string]*TokenCapability, len(capabilities))
	for _, tc := range capabilities {
		m[tc.Capability] = tc
	}
	return m, nil
}

func cacheDeleteTokenCapabilities(tokenId int) error {
	return common.RedisDel(capabilityCacheKey(tokenId))
}
