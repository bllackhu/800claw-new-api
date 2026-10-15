package controller

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type chatFileGrantRequest struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type chatFileSignRequest struct {
	ObjectKey string `json:"objectKey"`
}

func ChatFileUploadGrant(c *gin.Context) {
	tokenID := c.GetInt("token_id")
	if tokenID <= 0 {
		common.ApiErrorMsg(c, "token is required")
		return
	}
	var req chatFileGrantRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Size < 0 || req.Size > service.MaxChatFileBytes {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "file exceeds 1GB",
		})
		return
	}
	cfg, err := service.LoadChatOSSConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	grant, err := cfg.Grant(tokenID, req.Name, req.Size, time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, grant)
}

func ChatFileSign(c *gin.Context) {
	tokenID := c.GetInt("token_id")
	if tokenID <= 0 {
		common.ApiErrorMsg(c, "token is required")
		return
	}
	var req chatFileSignRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	cfg, err := service.LoadChatOSSConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	getURL, err := cfg.SignGet(req.ObjectKey, tokenID, time.Now())
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	common.ApiSuccess(c, gin.H{
		"objectKey": req.ObjectKey,
		"getUrl":    getURL,
	})
}
