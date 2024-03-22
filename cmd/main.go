package main

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"sync"
	"task/pkg/service"
	"time"
)

func main() {
	type FloodControl interface {
		// Check возвращает false если достигнут лимит максимально разрешенного
		// кол-ва запросов согласно заданным правилам флуд контроля.
		Check(ctx context.Context, userID int64) (bool, error)
	}

	ctx := context.Background()
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	defer client.Close()

	_, err := client.Ping(ctx).Result()
	if err != nil {
		fmt.Println("Error connecting to Redis:", err)
		return
	}

	config := service.RateLimitConfig{
		RequestsPerWindow: 5,
		Window:            time.Minute,
	}

	limiter := service.NewRateLimiter(config, client)
	go limiter.SyncToRedis(ctx)

	var wg sync.WaitGroup
	numUsers := 10
	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				allowed, err := limiter.Check(ctx, userID)
				if err != nil {
					fmt.Printf("Error for user %d: %v\n", userID, err)
					continue
				}
				if allowed {
					fmt.Printf("Request allowed for user %d\n", userID)
				} else {
					fmt.Printf("Request blocked for user %d\n", userID)
				}
				time.Sleep(time.Second)
			}
		}(int64(i))
	}

	wg.Wait()
}
