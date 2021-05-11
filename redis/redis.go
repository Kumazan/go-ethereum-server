package redis

import (
	"os"

	"github.com/go-redis/redis/v8"
)

var (
	redisAddr = os.Getenv("REDIS_ADDR")
)

func NewClient() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	return client
}
