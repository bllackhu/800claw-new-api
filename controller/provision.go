package controller

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const defaultProvisionTokenGroup = "default"

type provisionCustomerTokenRequest struct {
	CustomerUUID            string `json:"customer_uuid"`
	Name                    string `json:"name"`
	Group                   string `json:"group"`
	PoolID                  int    `json:"pool_id"`
	RequirePoolSubscription *bool  `json:"require_pool_subscription"`
}

func resolveProvisionTokenGroup(requested string) string {
	group := strings.TrimSpace(requested)
	if group == "" {
		return defaultProvisionTokenGroup
	}
	return group
}

func provisionTokenGroupAllowed(userGroup, tokenGroup string) bool {
	if tokenGroup == "auto" {
		return true
	}
	return service.GroupInUserUsableGroups(userGroup, tokenGroup)
}

func ProvisionCustomerToken(c *gin.Context) {
	var req provisionCustomerTokenRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	customerUUID := strings.TrimSpace(req.CustomerUUID)
	if customerUUID == "" {
		common.ApiErrorMsg(c, "customer_uuid is required")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		short := customerUUID
		if len(short) > 8 {
			short = short[:8]
		}
		name = "customer-" + short
	}
	if len(name) > 50 {
		common.ApiErrorMsg(c, "token name too long")
		return
	}

	userId := c.GetInt("id")
	requireSub := true
	if req.RequirePoolSubscription != nil {
		requireSub = *req.RequirePoolSubscription
	}
	group := resolveProvisionTokenGroup(req.Group)
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !provisionTokenGroupAllowed(userCache.Group, group) {
		common.ApiErrorMsg(c, fmt.Sprintf("无权访问 %s 分组", group))
		return
	}

	var existing model.Token
	err = model.DB.Where("user_id = ? AND name = ?", userId, name).First(&existing).Error
	if err == nil {
		if existing.Group == "" {
			existing.Group = group
			if err := existing.Update(); err != nil {
				common.ApiError(c, err)
				return
			}
		}
		if err := bindTokenPoolIfNeeded(&existing, req.PoolID); err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, gin.H{
			"key":      "sk-" + existing.Key,
			"token_id": existing.Id,
			"name":     existing.Name,
			"reused":   true,
		})
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiError(c, err)
		return
	}

	key, err := common.GenerateKey()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token := model.Token{
		UserId:                  userId,
		Name:                    name,
		Key:                     key,
		CreatedTime:             common.GetTimestamp(),
		AccessedTime:            common.GetTimestamp(),
		ExpiredTime:             -1,
		UnlimitedQuota:          true,
		Status:                  common.TokenStatusEnabled,
		RequirePoolSubscription: requireSub,
		TrialPeriodMonths:       1,
		Group:                   group,
	}
	if err := token.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := bindTokenPoolIfNeeded(&token, req.PoolID); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"key":      "sk-" + token.Key,
		"token_id": token.Id,
		"name":     token.Name,
		"reused":   false,
	})
}

func bindTokenPoolIfNeeded(token *model.Token, poolID int) error {
	if token == nil || token.Id <= 0 {
		return fmt.Errorf("token is not saved")
	}
	var pool *model.Pool
	var err error
	if poolID > 0 {
		pool, err = model.GetPoolById(poolID)
	} else {
		pool, err = model.GetDefaultPool()
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if pool == nil {
		return nil
	}
	binding := &model.PoolBinding{
		BindingType:  model.PoolBindingTypeToken,
		BindingValue: strconv.Itoa(token.Id),
		PoolId:       pool.Id,
		Priority:     0,
		Enabled:      true,
		CreatedAt:    common.GetTimestamp(),
		UpdatedAt:    common.GetTimestamp(),
	}
	if err := model.CreatePoolBinding(binding); err != nil && !strings.Contains(err.Error(), "duplicate") {
		return err
	}
	return nil
}
