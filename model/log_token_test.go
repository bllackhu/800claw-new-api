package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func seedTokenLog(t *testing.T, tokenID int, userID int, logType int, modelName, tokenName string, createdAt int64) {
	t.Helper()
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:           userID,
		TokenId:          tokenID,
		CreatedAt:        createdAt,
		Type:             logType,
		TokenName:        tokenName,
		ModelName:        modelName,
		Username:         "tester",
		PromptTokens:     10,
		CompletionTokens: 5,
		Other:            `{"frt":120,"cache_tokens":3}`,
	}).Error)
}

func TestGetLogsByTokenId_Pagination(t *testing.T) {
	truncateTables(t)

	tokenID := 55
	for i := 0; i < 5; i++ {
		seedTokenLog(t, tokenID, 42, LogTypeConsume, "gpt-4o-mini", "tkn-a", int64(1700000000+i))
	}

	page1, total, err := GetLogsByTokenId(tokenID, LogTypeUnknown, 0, 0, "", 0, 2)
	require.NoError(t, err)
	require.EqualValues(t, 5, total)
	require.Len(t, page1, 2)
	// newest first (highest created_at / highest id first)
	require.Equal(t, "gpt-4o-mini", page1[0].ModelName)

	page2, _, err := GetLogsByTokenId(tokenID, LogTypeUnknown, 0, 0, "", 2, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)

	page3, _, err := GetLogsByTokenId(tokenID, LogTypeUnknown, 0, 0, "", 4, 2)
	require.NoError(t, err)
	require.Len(t, page3, 1)
}

func TestGetLogsByTokenId_TokenScoping(t *testing.T) {
	truncateTables(t)

	seedTokenLog(t, 100, 42, LogTypeConsume, "gpt-4o", "tkn-a", 1700000000)
	seedTokenLog(t, 200, 42, LogTypeConsume, "gpt-4o", "tkn-b", 1700000000)

	logs, total, err := GetLogsByTokenId(100, LogTypeUnknown, 0, 0, "", 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	require.Equal(t, "tkn-a", logs[0].TokenName)
}

func TestGetLogsByTokenId_ModelAndTypeFilters(t *testing.T) {
	truncateTables(t)

	tokenID := 77
	seedTokenLog(t, tokenID, 42, LogTypeConsume, "gpt-4o-mini", "tkn-a", 1700000000)
	seedTokenLog(t, tokenID, 42, LogTypeConsume, "claude-3-5-sonnet", "tkn-a", 1700000001)
	seedTokenLog(t, tokenID, 42, LogTypeError, "gpt-4o-mini", "tkn-a", 1700000002)

	t.Run("model substring", func(t *testing.T) {
		logs, total, err := GetLogsByTokenId(tokenID, LogTypeUnknown, 0, 0, "claude", 0, 10)
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, logs, 1)
		require.Equal(t, "claude-3-5-sonnet", logs[0].ModelName)
	})

	t.Run("type consume only", func(t *testing.T) {
		logs, total, err := GetLogsByTokenId(tokenID, LogTypeConsume, 0, 0, "", 0, 10)
		require.NoError(t, err)
		require.EqualValues(t, 2, total)
		require.Len(t, logs, 2)
	})

	t.Run("time range", func(t *testing.T) {
		logs, total, err := GetLogsByTokenId(tokenID, LogTypeUnknown, 1700000000, 1700000001, "", 0, 10)
		require.NoError(t, err)
		require.EqualValues(t, 2, total)
		require.Len(t, logs, 2)
	})
}

func TestGetLogsByTokenId_SanitizesOther(t *testing.T) {
	truncateTables(t)

	tokenID := 88
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:           42,
		TokenId:          tokenID,
		CreatedAt:        1700000000,
		Type:             LogTypeConsume,
		TokenName:        "tkn-a",
		ModelName:        "gpt-4o",
		Username:         "tester",
		PromptTokens:     10,
		CompletionTokens: 5,
		ChannelName:      "secret-channel",
		Other:            `{"frt":120,"cache_tokens":3,"admin_info":{"use_channel":["ch-1"]},"stream_status":{"status":"ok"}}`,
	}).Error)

	logs, _, err := GetLogsByTokenId(tokenID, LogTypeUnknown, 0, 0, "", 0, 10)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Empty(t, logs[0].ChannelName)
	other, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	require.NotContains(t, other, "admin_info")
	require.NotContains(t, other, "stream_status")
	require.Equal(t, float64(120), other["frt"])
	require.Equal(t, float64(3), other["cache_tokens"])
}
