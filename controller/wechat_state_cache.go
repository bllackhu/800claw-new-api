package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type stateEntry struct {
	TokenId          int
	PoolId           int
	PeriodMonths     int
	UpgradeFromPoolId int
	ExpiresAt        time.Time
}

const (
	stateTokenTTL      = 10 * time.Minute
	stateTokenRedisKey = "new-api:jsapi_state:"
)

var (
	stateCacheMu sync.RWMutex
	stateCache   = make(map[string]*stateEntry)
)

func generateStateToken(tokenId, poolId, periodMonths, upgradeFromPoolId int) string {
	b := make([]byte, 16)
	rand.Read(b)
	token := hex.EncodeToString(b)

	if common.RedisEnabled && common.RDB != nil {
		ctx := context.Background()
		key := stateTokenRedisKey + token
		val := fmt.Sprintf("%d:%d:%d:%d", tokenId, poolId, periodMonths, upgradeFromPoolId)
		common.RDB.Set(ctx, key, val, stateTokenTTL)
	} else {
		stateCacheMu.Lock()
		stateCache[token] = &stateEntry{
			TokenId:           tokenId,
			PoolId:            poolId,
			PeriodMonths:      periodMonths,
			UpgradeFromPoolId: upgradeFromPoolId,
			ExpiresAt:         time.Now().Add(stateTokenTTL),
		}
		stateCacheMu.Unlock()
	}

	return token
}

func consumeStateToken(token string) (tokenId, poolId, periodMonths, upgradeFromPoolId int, ok bool) {
	if common.RedisEnabled && common.RDB != nil {
		ctx := context.Background()
		key := stateTokenRedisKey + token
		val, err := common.RDB.Get(ctx, key).Result()
		if err == nil && val != "" {
			var tid, pid, pm, upid int
			if _, scanErr := fmt.Sscanf(val, "%d:%d:%d:%d", &tid, &pid, &pm, &upid); scanErr == nil {
				return tid, pid, pm, upid, true
			}
			// backward compatibility: old format without upgrade_from_pool_id
			if _, scanErr := fmt.Sscanf(val, "%d:%d:%d", &tid, &pid, &pm); scanErr == nil {
				return tid, pid, pm, 0, true
			}
		}
		return 0, 0, 0, 0, false
	}

	stateCacheMu.Lock()
	defer stateCacheMu.Unlock()

	entry, found := stateCache[token]
	if !found {
		return 0, 0, 0, 0, false
	}

	if time.Now().After(entry.ExpiresAt) {
		return 0, 0, 0, 0, false
	}

	return entry.TokenId, entry.PoolId, entry.PeriodMonths, entry.UpgradeFromPoolId, true
}

func init() {
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			stateCacheMu.Lock()
			now := time.Now()
			for k, v := range stateCache {
				if now.After(v.ExpiresAt) {
					delete(stateCache, k)
				}
			}
			stateCacheMu.Unlock()
		}
	}()
}