package model

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

// TokenLLMUsageBucketSeconds is the fixed rollup width (1 hour) for LLM token buckets.
const TokenLLMUsageBucketSeconds int64 = 3600

// TokenLLMUsageBucket stores per-hour LLM token sums for a single API token (main DB).
type TokenLLMUsageBucket struct {
	TokenId          int   `json:"token_id" gorm:"primaryKey;not null"`
	BucketStart      int64 `json:"bucket_start" gorm:"primaryKey;bigint;not null"`
	PromptTokens     int64 `json:"prompt_tokens" gorm:"bigint;default:0"`
	CompletionTokens int64 `json:"completion_tokens" gorm:"bigint;default:0"`
	RequestCount     int64 `json:"request_count" gorm:"bigint;default:0"`
}

func (TokenLLMUsageBucket) TableName() string {
	return "token_llm_usage_buckets"
}

// AlignTokenLLMUsageBucketStart returns the unix start of the hour bucket for ts.
func AlignTokenLLMUsageBucketStart(ts int64) int64 {
	if ts <= 0 {
		return 0
	}
	return ts - (ts % TokenLLMUsageBucketSeconds)
}

func isUniqueConstraintViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique constraint") ||
		strings.Contains(s, "duplicate entry") ||
		strings.Contains(s, "constraint failed")
}

// UpsertTokenLLMUsageBucket increments counters for (token_id, bucket_start), creating the row if missing.
func UpsertTokenLLMUsageBucket(tokenId int, bucketStart int64, dPrompt, dCompletion, dRequests int64) error {
	if DB == nil || tokenId <= 0 || bucketStart <= 0 {
		return nil
	}
	if dPrompt == 0 && dCompletion == 0 && dRequests == 0 {
		return nil
	}

	for attempt := 0; attempt < 2; attempt++ {
		res := DB.Model(&TokenLLMUsageBucket{}).
			Where("token_id = ? AND bucket_start = ?", tokenId, bucketStart).
			Updates(map[string]interface{}{
				"prompt_tokens":       gorm.Expr("COALESCE(prompt_tokens, 0) + ?", dPrompt),
				"completion_tokens": gorm.Expr("COALESCE(completion_tokens, 0) + ?", dCompletion),
				"request_count":       gorm.Expr("COALESCE(request_count, 0) + ?", dRequests),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			return nil
		}

		row := TokenLLMUsageBucket{
			TokenId:          tokenId,
			BucketStart:      bucketStart,
			PromptTokens:     dPrompt,
			CompletionTokens: dCompletion,
			RequestCount:     dRequests,
		}
		err := DB.Create(&row).Error
		if err == nil {
			return nil
		}
		if isUniqueConstraintViolation(err) {
			continue
		}
		return err
	}
	return errors.New("upsert token_llm_usage_buckets failed after retry")
}

// SumTokenLLMUsageBucketsByTokenSince sums prompt/completion from buckets with bucket_start >= sinceUnix.
// If sinceUnix <= 0, sums all buckets for the token.
func SumTokenLLMUsageBucketsByTokenSince(tokenId int, sinceUnix int64) (promptSum int64, completionSum int64, err error) {
	if DB == nil || tokenId <= 0 {
		return 0, 0, nil
	}
	type aggRow struct {
		PromptSum     int64 `gorm:"column:prompt_sum"`
		CompletionSum int64 `gorm:"column:completion_sum"`
	}
	var row aggRow
	q := DB.Model(&TokenLLMUsageBucket{}).
		Select("COALESCE(SUM(prompt_tokens), 0) AS prompt_sum, COALESCE(SUM(completion_tokens), 0) AS completion_sum").
		Where("token_id = ?", tokenId)
	if sinceUnix > 0 {
		q = q.Where("bucket_start >= ?", sinceUnix)
	}
	err = q.Scan(&row).Error
	if err != nil {
		return 0, 0, err
	}
	return row.PromptSum, row.CompletionSum, nil
}

// SumTokenLLMUsageBucketRequestCountByTokenSince sums request_count from buckets with bucket_start >= sinceUnix.
func SumTokenLLMUsageBucketRequestCountByTokenSince(tokenId int, sinceUnix int64) (int64, error) {
	if DB == nil || tokenId <= 0 {
		return 0, nil
	}
	var total int64
	q := DB.Model(&TokenLLMUsageBucket{}).
		Select("COALESCE(SUM(request_count), 0)").
		Where("token_id = ?", tokenId)
	if sinceUnix > 0 {
		q = q.Where("bucket_start >= ?", sinceUnix)
	}
	err := q.Scan(&total).Error
	return total, err
}

// DeleteTokenLLMUsageBucketsBefore deletes bucket rows with bucket_start < beforeUnix in batches of limit.
func DeleteTokenLLMUsageBucketsBefore(ctx context.Context, beforeUnix int64, limit int) (int64, error) {
	if DB == nil || beforeUnix <= 0 || limit <= 0 {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var total int64
	for {
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
		result := DB.WithContext(ctx).Where("bucket_start < ?", beforeUnix).Limit(limit).Delete(&TokenLLMUsageBucket{})
		if result.Error != nil {
			return total, result.Error
		}
		total += result.RowsAffected
		if result.RowsAffected < int64(limit) {
			break
		}
	}
	return total, nil
}
