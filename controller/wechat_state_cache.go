package controller

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type stateEntry struct {
	TokenId      int
	PoolId       int
	PeriodMonths int
	ExpiresAt    time.Time
}

var (
	stateCacheMu sync.RWMutex
	stateCache   = make(map[string]*stateEntry)
)

const stateTokenTTL = 10 * time.Minute

func generateStateToken(tokenId, poolId, periodMonths int) string {
	b := make([]byte, 16)
	rand.Read(b)
	token := hex.EncodeToString(b)

	stateCacheMu.Lock()
	stateCache[token] = &stateEntry{
		TokenId:      tokenId,
		PoolId:       poolId,
		PeriodMonths: periodMonths,
		ExpiresAt:    time.Now().Add(stateTokenTTL),
	}
	stateCacheMu.Unlock()

	return token
}

func consumeStateToken(token string) (tokenId, poolId, periodMonths int, ok bool) {
	stateCacheMu.Lock()
	defer stateCacheMu.Unlock()

	entry, found := stateCache[token]
	if !found {
		return 0, 0, 0, false
	}

	delete(stateCache, token)

	if time.Now().After(entry.ExpiresAt) {
		return 0, 0, 0, false
	}

	return entry.TokenId, entry.PoolId, entry.PeriodMonths, true
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