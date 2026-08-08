package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type logPageResponse struct {
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Total    int              `json:"total"`
	Items    []map[string]any `json:"items"`
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func setupLogTokenControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedTokenLogForController(t *testing.T, db *gorm.DB, tokenID int, userID int, modelName string, createdAt int64) {
	t.Helper()
	require.NoError(t, db.Create(&model.Log{
		UserId:           userID,
		TokenId:          tokenID,
		CreatedAt:        createdAt,
		Type:             model.LogTypeConsume,
		TokenName:        "tkn-a",
		ModelName:        modelName,
		Username:         "tester",
		PromptTokens:     10,
		CompletionTokens: 5,
		Other:            `{"frt":120,"cache_tokens":3}`,
	}).Error)
}

func newTokenLogContext(t *testing.T, target string, tokenID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	ctx.Set("token_id", tokenID)
	return ctx, recorder
}

func TestGetLogsByToken_ReturnsPaginatedSelfLogs(t *testing.T) {
	db := setupLogTokenControllerTestDB(t)
	for i := 0; i < 5; i++ {
		seedTokenLogForController(t, db, 300, 42, "gpt-4o-mini", int64(1700000000+i))
	}
	// another token's logs must not leak
	seedTokenLogForController(t, db, 400, 42, "claude-3-5-sonnet", 1700000000)

	ctx, recorder := newTokenLogContext(t, "/api/usage/token/logs?p=1&page_size=2", 300)
	GetLogsByToken(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.True(t, envelope.Success)

	var page logPageResponse
	require.NoError(t, common.Unmarshal(envelope.Data, &page))
	require.Equal(t, 1, page.Page)
	require.Equal(t, 2, page.PageSize)
	require.Equal(t, 5, page.Total)
	require.Len(t, page.Items, 2)
	for _, item := range page.Items {
		require.Equal(t, "gpt-4o-mini", item["model_name"])
		require.Equal(t, "tkn-a", item["token_name"])
	}
}

func TestGetLogsByToken_ModelFilter(t *testing.T) {
	db := setupLogTokenControllerTestDB(t)
	seedTokenLogForController(t, db, 300, 42, "gpt-4o-mini", 1700000000)
	seedTokenLogForController(t, db, 300, 42, "claude-3-5-sonnet", 1700000001)

	ctx, recorder := newTokenLogContext(t, "/api/usage/token/logs?p=1&page_size=10&model_name=claude", 300)
	GetLogsByToken(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.True(t, envelope.Success)

	var page logPageResponse
	require.NoError(t, common.Unmarshal(envelope.Data, &page))
	require.Equal(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	require.Equal(t, "claude-3-5-sonnet", page.Items[0]["model_name"])
}
