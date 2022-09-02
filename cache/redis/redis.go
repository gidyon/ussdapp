package redis

import (
	"context"
	"time"

	"github.com/gidyon/ussdapp"
	"github.com/go-redis/redis/v8"
)

func NewRedisCache(opt *Options, expireDur time.Duration) ussdapp.Cacher {
	redisClient := redis.NewClient(&redis.Options{
		Network:            "tcp",
		Addr:               opt.Addr,
		Username:           opt.Username,
		Password:           opt.Password,
		DB:                 opt.DB,
		MaxRetries:         opt.MaxRetries,
		MinRetryBackoff:    opt.MinRetryBackoff,
		MaxRetryBackoff:    opt.MaxRetryBackoff,
		DialTimeout:        opt.DialTimeout,
		ReadTimeout:        opt.ReadTimeout,
		WriteTimeout:       opt.WriteTimeout,
		PoolFIFO:           opt.PoolFIFO,
		PoolSize:           opt.PoolSize,
		MinIdleConns:       opt.MinIdleConns,
		MaxConnAge:         opt.MaxConnAge,
		PoolTimeout:        opt.PoolTimeout,
		IdleTimeout:        opt.IdleTimeout,
		IdleCheckFrequency: opt.IdleCheckFrequency,
	})

	rc := &redisCache{cc: redisClient}

	return rc
}

type redisCache struct {
	cc  *redis.Client
	dur time.Duration
}

func (rc *redisCache) Set(ctx context.Context, key, value string) error {
	return rc.cc.Set(ctx, key, value, rc.dur).Err()
}

func (rc *redisCache) Get(ctx context.Context, key string) (string, error) {
	return rc.cc.Get(ctx, key).Result()
}

func (rc *redisCache) Delete(ctx context.Context, key, value string) error {
	return rc.cc.Del(ctx, key).Err()
}

func (rc *redisCache) SetMap(ctx context.Context, key string, fields map[string]interface{}) error {
	return rc.cc.HSet(ctx, key, fields).Err()
}

func (rc *redisCache) GetMap(ctx context.Context, key string) (map[string]string, error) {
	return rc.cc.HGetAll(ctx, key).Result()
}

func (rc *redisCache) DeleteMap(ctx context.Context, key string) error {
	return rc.cc.Del(ctx, key).Err()
}

func (rc *redisCache) SetMapField(ctx context.Context, key string, values ...interface{}) error {
	return rc.cc.HSet(ctx, key, values...).Err()
}

func (rc *redisCache) GetMapField(ctx context.Context, key, field string) (string, error) {
	return rc.cc.HGet(ctx, key, field).Result()
}

func (rc *redisCache) DeleteMapField(ctx context.Context, key string, fields ...string) error {
	return rc.cc.HDel(ctx, key, fields...).Err()
}
