package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPoolRollingRequestRedisKeys(t *testing.T) {
	require.Equal(t, "token:rq:events:42", TokenRollingRequestRedisKey(42))
	require.Equal(t, "pool:rq:events:3:token:42", LegacyPoolTokenRollingRequestRedisKey(3, 42))
	require.Equal(t, "token:rq:events:9", PoolRollingRequestRedisKey(1, "token:9"))
	require.Equal(t, "pool:rq:events:1:user:5", PoolRollingRequestRedisKey(1, "user:5"))
}

func TestSumTokenLLMUsageBucketRequestCountByTokenSince(t *testing.T) {
	truncateTables(t)

	tokenID := 501
	now := int64(1_700_000_000)
	require.NoError(t, DB.Create(&TokenLLMUsageBucket{
		TokenId:      tokenID,
		BucketStart:  now - 7200,
		RequestCount: 3,
	}).Error)
	require.NoError(t, DB.Create(&TokenLLMUsageBucket{
		TokenId:      tokenID,
		BucketStart:  now - 48*3600,
		RequestCount: 10,
	}).Error)

	total, err := SumTokenLLMUsageBucketRequestCountByTokenSince(tokenID, now-24*3600)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)

	all, err := SumTokenLLMUsageBucketRequestCountByTokenSince(tokenID, 0)
	require.NoError(t, err)
	require.EqualValues(t, 13, all)
}

func TestMergeTokenRollingRequestEvents_NoOpWhenRedisDisabled(t *testing.T) {
	require.NoError(t, MergeTokenRollingRequestEvents(1, 42, 3600))
}
