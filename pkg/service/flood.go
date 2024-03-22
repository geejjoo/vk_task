package service

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"sync"

	"time"
)

type FloodControl interface {
	Check(ctx context.Context, userID int64) (bool, error)
}

type RateLimiter struct {
	config     RateLimitConfig
	windowSize time.Duration
	counts     map[int64][]time.Time
	mu         sync.Mutex
	client     *redis.Client
}

type RateLimitConfig struct {
	RequestsPerWindow int
	Window            time.Duration
}

func NewRateLimiter(config RateLimitConfig, client *redis.Client) *RateLimiter {
	return &RateLimiter{
		config:     config,
		windowSize: config.Window,
		counts:     make(map[int64][]time.Time),
		client:     client,
	}
}

func (rl *RateLimiter) Check(ctx context.Context, userID int64) (bool, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Truncate(rl.windowSize)
	counts, exists := rl.counts[userID]

	if !exists || len(counts) == 0 || counts[0].Before(windowStart) {
		counts = []time.Time{now}
	} else {
		counts = append(counts, now)
	}

	cutoff := now.Add(-rl.windowSize)
	i := 0
	for i < len(counts) && counts[i].Before(cutoff) {
		i++
	}
	counts = counts[i:]

	if len(counts) > rl.config.RequestsPerWindow {
		return false, nil
	}

	rl.counts[userID] = counts
	return true, nil
}

func (rl *RateLimiter) SyncToRedis(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rl.mu.Lock()
			for userID, counts := range rl.counts {
				key := fmt.Sprintf("ratelimit:%d", userID)
				err := rl.client.Set(ctx, key, len(counts), 0).Err()
				if err != nil {
					fmt.Printf("Error syncing data to Redis for user %d: %v\n", userID, err)
				}
			}
			rl.mu.Unlock()
		}
	}
}
