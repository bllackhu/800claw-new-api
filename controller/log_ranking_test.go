package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newTokenRankingContext(t *testing.T, target string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return ctx, recorder
}

type tokenRankingResponse struct {
	Ranking  []model.TokenRequestRank      `json:"ranking"`
	Items    []model.TokenRequestRank      `json:"items"`
	Page     int                           `json:"page"`
	PageSize int                           `json:"page_size"`
	Total    model.TokenRequestRankTotal   `json:"total"`
}

func parseTokenRanking(t *testing.T, recorder *httptest.ResponseRecorder) tokenRankingResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.True(t, envelope.Success)
	var data tokenRankingResponse
	require.NoError(t, common.Unmarshal(envelope.Data, &data))
	return data
}

func TestGetTopTokenRequestCounts_SortedByCountWithLimit(t *testing.T) {
	db := setupLogTokenControllerTestDB(t)
	for i := 0; i < 3; i++ {
		seedTokenLogForController(t, db, 100, 42, "gpt-4o-mini", int64(1700000000+i))
	}
	for i := 0; i < 2; i++ {
		seedTokenLogForController(t, db, 200, 42, "claude-3-5-sonnet", int64(1700000000+i))
	}
	seedTokenLogForController(t, db, 300, 43, "gemini-1.5-pro", 1700000000)
	// non-consume logs must not count toward request counts
	require.NoError(t, db.Create(&model.Log{
		UserId:    42,
		TokenId:   100,
		CreatedAt: 1700000000,
		Type:      model.LogTypeTopup,
		TokenName: "tkn-a",
		ModelName: "gpt-4o-mini",
		Username:  "tester",
	}).Error)

	ctx, recorder := newTokenRankingContext(t, "/api/log/token_ranking?limit=2")
	GetTopTokenRequestCounts(ctx)

	data := parseTokenRanking(t, recorder)
	require.Len(t, data.Ranking, 2)
	require.Equal(t, int64(3), data.Ranking[0].RequestCount)
	require.Equal(t, 100, data.Ranking[0].TokenId)
	require.Equal(t, int64(2), data.Ranking[1].RequestCount)
	require.Equal(t, 200, data.Ranking[1].TokenId)
	require.Equal(t, int64(6), data.Total.RequestCount)
	require.Equal(t, int64(3), data.Total.TokenCount)
}

func TestGetTopTokenRequestCounts_TokenFilter(t *testing.T) {
	db := setupLogTokenControllerTestDB(t)
	seedTokenLogForController(t, db, 100, 42, "gpt-4o-mini", 1700000000)
	seedTokenLogForController(t, db, 100, 42, "gpt-4o-mini", 1700000005)
	// distinct token name so the token_name filter can distinguish rows
	require.NoError(t, db.Create(&model.Log{
		UserId:           42,
		TokenId:          200,
		CreatedAt:        1700000000,
		Type:             model.LogTypeConsume,
		TokenName:        "claw-b",
		ModelName:        "claude-3-5-sonnet",
		Username:         "tester",
		PromptTokens:     10,
		CompletionTokens: 5,
	}).Error)

	ctx, recorder := newTokenRankingContext(t, "/api/log/token_ranking?token_name=tkn")
	GetTopTokenRequestCounts(ctx)

	data := parseTokenRanking(t, recorder)
	require.Len(t, data.Ranking, 1)
	require.Equal(t, 100, data.Ranking[0].TokenId)
	require.Equal(t, int64(2), data.Ranking[0].RequestCount)
	require.Equal(t, int64(2), data.Total.RequestCount)
	require.Equal(t, int64(1), data.Total.TokenCount)
}

func TestGetTopTokenRequestCounts_TimeFilter(t *testing.T) {
	db := setupLogTokenControllerTestDB(t)
	seedTokenLogForController(t, db, 100, 42, "gpt-4o-mini", 1700000000)
	seedTokenLogForController(t, db, 100, 42, "gpt-4o-mini", 1700000005)
	seedTokenLogForController(t, db, 200, 42, "claude-3-5-sonnet", 1700000000)

	ctx, recorder := newTokenRankingContext(t, "/api/log/token_ranking?start_timestamp=1700000003")
	GetTopTokenRequestCounts(ctx)

	data := parseTokenRanking(t, recorder)
	require.Len(t, data.Ranking, 1)
	require.Equal(t, int64(1), data.Ranking[0].RequestCount)
	require.Equal(t, int64(1), data.Total.RequestCount)
	require.Equal(t, int64(1), data.Total.TokenCount)
}

func TestGetTopTokenRequestCounts_DefaultLimit(t *testing.T) {
	db := setupLogTokenControllerTestDB(t)
	for i := 0; i < 15; i++ {
		seedTokenLogForController(t, db, 1000+i, 42, "gpt-4o-mini", 1700000000)
	}

	ctx, recorder := newTokenRankingContext(t, "/api/log/token_ranking")
	GetTopTokenRequestCounts(ctx)

	data := parseTokenRanking(t, recorder)
	require.Len(t, data.Ranking, 10)
	require.Equal(t, int64(15), data.Total.RequestCount)
	require.Equal(t, int64(15), data.Total.TokenCount)
}

func TestGetTopTokenRequestCounts_ModelCount(t *testing.T) {
	db := setupLogTokenControllerTestDB(t)
	// token 100 uses two distinct models within the window
	seedTokenLogForController(t, db, 100, 42, "gpt-4o-mini", 1700000000)
	seedTokenLogForController(t, db, 100, 42, "gpt-4o-mini", 1700000005)
	seedTokenLogForController(t, db, 100, 42, "claude-3-5-sonnet", 1700000006)
	// token 200 uses a single model
	seedTokenLogForController(t, db, 200, 42, "gemini-1.5-pro", 1700000000)

	ctx, recorder := newTokenRankingContext(t, "/api/log/token_ranking")
	GetTopTokenRequestCounts(ctx)

	data := parseTokenRanking(t, recorder)
	require.Len(t, data.Ranking, 2)
	require.Equal(t, 100, data.Ranking[0].TokenId)
	require.Equal(t, int64(2), data.Ranking[0].ModelCount)
	require.Len(t, data.Ranking[0].Models, 2)
	require.Equal(t, "gpt-4o-mini", data.Ranking[0].Models[0].ModelName)
	require.Equal(t, int64(2), data.Ranking[0].Models[0].RequestCount)
	require.Equal(t, "claude-3-5-sonnet", data.Ranking[0].Models[1].ModelName)
	require.Equal(t, int64(1), data.Ranking[0].Models[1].RequestCount)
	require.Equal(t, 200, data.Ranking[1].TokenId)
	require.Equal(t, int64(1), data.Ranking[1].ModelCount)
	require.Len(t, data.Ranking[1].Models, 1)
	require.Equal(t, "gemini-1.5-pro", data.Ranking[1].Models[0].ModelName)
}

func TestGetTopTokenRequestCounts_ModelTieBreak(t *testing.T) {
	db := setupLogTokenControllerTestDB(t)
	// equal request counts per model → alphabetical by model name
	seedTokenLogForController(t, db, 100, 42, "zz-model", 1700000000)
	seedTokenLogForController(t, db, 100, 42, "aa-model", 1700000005)
	seedTokenLogForController(t, db, 100, 42, "mm-model", 1700000006)

	ctx, recorder := newTokenRankingContext(t, "/api/log/token_ranking")
	GetTopTokenRequestCounts(ctx)

	data := parseTokenRanking(t, recorder)
	require.Len(t, data.Ranking, 1)
	require.Equal(t, int64(3), data.Ranking[0].ModelCount)
	require.Len(t, data.Ranking[0].Models, 3)
	require.Equal(t, "aa-model", data.Ranking[0].Models[0].ModelName)
	require.Equal(t, "mm-model", data.Ranking[0].Models[1].ModelName)
	require.Equal(t, "zz-model", data.Ranking[0].Models[2].ModelName)
}

func TestGetTopTokenRequestCounts_Pagination(t *testing.T) {
	db := setupLogTokenControllerTestDB(t)
	// distinct request counts so the DESC ordering is deterministic
	for i := 0; i < 5; i++ {
		for j := 0; j <= i; j++ {
			seedTokenLogForController(t, db, 1000+i, 42, "gpt-4o-mini", int64(1700000000+j))
		}
	}

	ctx, recorder := newTokenRankingContext(t, "/api/log/token_ranking?page=1&page_size=2")
	GetTopTokenRequestCounts(ctx)

	data := parseTokenRanking(t, recorder)
	require.Len(t, data.Items, 2)
	require.Equal(t, 1, data.Page)
	require.Equal(t, 2, data.PageSize)
	require.Equal(t, int64(5), data.Total.TokenCount)
	// chart ranking still returns top-10 (all 5 tokens here)
	require.Len(t, data.Ranking, 5)
	require.Equal(t, 1004, data.Items[0].TokenId)
	require.Equal(t, 1003, data.Items[1].TokenId)
	// paginated items carry per-model breakdown too
	require.Equal(t, int64(1), data.Items[0].ModelCount)
	require.Len(t, data.Items[0].Models, 1)

	ctx2, recorder2 := newTokenRankingContext(t, "/api/log/token_ranking?page=3&page_size=2")
	GetTopTokenRequestCounts(ctx2)

	data2 := parseTokenRanking(t, recorder2)
	require.Len(t, data2.Items, 1)
	require.Equal(t, 1000, data2.Items[0].TokenId)
}
