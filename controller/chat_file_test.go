package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func chatFileContext(t *testing.T, method, target, body string, tokenID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("ALIYUN_OSS_ENDPOINT", "oss-cn-hangzhou.aliyuncs.com")
	t.Setenv("ALIYUN_OSS_BUCKET", "bucket")
	t.Setenv("ALIYUN_OSS_ACCESS_KEY_ID", "id")
	t.Setenv("ALIYUN_OSS_ACCESS_KEY_SECRET", "secret")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("token_id", tokenID)
	return ctx, recorder
}

func TestChatFileUploadGrantUsesTokenID(t *testing.T) {
	ctx, recorder := chatFileContext(t, http.MethodPost, "/api/chat/files/upload-grant", `{"name":"hello.docx","size":12}`, 7)
	ChatFileUploadGrant(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                  `json:"success"`
		Data    service.ChatFileGrant `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.True(t, strings.HasPrefix(response.Data.ObjectKey, "chat-temp/7/"))
	require.NotContains(t, response.Data.ObjectKey, "sk-")
	require.Contains(t, response.Data.PutURL, "OSSAccessKeyId=id")
}

func TestChatFileUploadGrantRejectsOver1GB(t *testing.T) {
	body := `{"name":"big.bin","size":1073741825}`
	ctx, recorder := chatFileContext(t, http.MethodPost, "/api/chat/files/upload-grant", body, 7)
	ChatFileUploadGrant(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
	require.Equal(t, "file exceeds 1GB", response.Message)
}

func TestChatFileSignRejectsAnotherToken(t *testing.T) {
	ctx, recorder := chatFileContext(
		t,
		http.MethodPost,
		"/api/chat/files/sign",
		`{"objectKey":"chat-temp/8/2026/10/14/abc/a.docx"}`,
		7,
	)
	ChatFileSign(ctx)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
}
