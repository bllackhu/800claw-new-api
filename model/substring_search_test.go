package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func init() {
	initCol()
}

func TestSubstringLikePattern(t *testing.T) {
	t.Run("auto wrap substring", func(t *testing.T) {
		pattern, err := substringLikePattern("minimax")
		require.NoError(t, err)
		require.Equal(t, "%minimax%", pattern)
	})

	t.Run("preserve explicit prefix pattern", func(t *testing.T) {
		pattern, err := substringLikePattern("gpt%")
		require.NoError(t, err)
		require.Equal(t, "gpt%", pattern)
	})

	t.Run("escape underscore", func(t *testing.T) {
		pattern, err := substringLikePattern("a_b")
		require.NoError(t, err)
		require.Equal(t, "%a!_b%", pattern)
	})

	t.Run("reject single char fuzzy", func(t *testing.T) {
		_, err := substringLikePattern("a")
		require.Error(t, err)
	})

	t.Run("reject double percent", func(t *testing.T) {
		_, err := substringLikePattern("a%%b")
		require.Error(t, err)
	})
}

func TestSearchUserTokensSubstringMatch(t *testing.T) {
	truncateTables(t)

	userID := 1
	keys := []string{
		"0123456789012345678901234567890123456789012345678",
		"1123456789012345678901234567890123456789012345678",
	}
	names := []string{"local-dashscope", "local-minimax"}
	for i, name := range names {
		require.NoError(t, DB.Create(&Token{
			UserId:         userID,
			Name:           name,
			Key:            keys[i],
			Status:         1,
			CreatedTime:    1,
			AccessedTime:   1,
			ExpiredTime:    -1,
			RemainQuota:    1,
			UnlimitedQuota: true,
			Group:          "default",
		}).Error)
	}

	t.Run("name substring", func(t *testing.T) {
		tokens, total, err := SearchUserTokens(userID, "dash", "", 0, 10)
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, tokens, 1)
		require.Equal(t, "local-dashscope", tokens[0].Name)
	})

	t.Run("key substring", func(t *testing.T) {
		tokens, total, err := SearchUserTokens(userID, "", "2345678901", 0, 10)
		require.NoError(t, err)
		require.EqualValues(t, 2, total)
		require.Len(t, tokens, 2)
	})

	t.Run("full name still matches", func(t *testing.T) {
		tokens, total, err := SearchUserTokens(userID, "local-minimax", "", 0, 10)
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, tokens, 1)
		require.Equal(t, "local-minimax", tokens[0].Name)
	})
}

func seedUsageLog(t *testing.T, userID int, tokenName, modelName string) {
	t.Helper()
	now := time.Now().Unix()
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:           userID,
		CreatedAt:        now,
		Type:             LogTypeConsume,
		TokenName:        tokenName,
		ModelName:        modelName,
		Username:         "tester",
		PromptTokens:     10,
		CompletionTokens: 5,
	}).Error)
}

func TestGetUserLogsSubstringMatch(t *testing.T) {
	truncateTables(t)

	userID := 42
	seedUsageLog(t, userID, "local-dashscope", "qwen-max")
	seedUsageLog(t, userID, "local-minimax", "gpt-4o-mini")

	t.Run("token name substring", func(t *testing.T) {
		logs, total, err := GetUserLogs(userID, LogTypeUnknown, 0, 0, "", "mini", 0, 10, "", "")
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, logs, 1)
		require.Equal(t, "local-minimax", logs[0].TokenName)
	})

	t.Run("model name substring", func(t *testing.T) {
		logs, total, err := GetUserLogs(userID, LogTypeUnknown, 0, 0, "qwen", "", 0, 10, "", "")
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, logs, 1)
		require.Equal(t, "qwen-max", logs[0].ModelName)
	})
}

func TestGetAllLogsSubstringMatch(t *testing.T) {
	truncateTables(t)

	userID := 7
	seedUsageLog(t, userID, "prod-token", "claude-3-5-sonnet")

	logs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "claude", "", "prod", 0, 10, 0, "", "")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	require.Equal(t, "prod-token", logs[0].TokenName)
	require.Equal(t, "claude-3-5-sonnet", logs[0].ModelName)
}
