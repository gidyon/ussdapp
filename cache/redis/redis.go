package redis

import (
	"context"
	"errors"
	"time"

	"github.com/gidyon/ussdapp"
	"github.com/go-redis/redis/v8"
)

// NewRedisCache creates a USSD cacher using redis
func NewRedisCache(opt *redis.Options) ussdapp.Cacher {
	redisClient := redis.NewClient(opt)
	rc := &redisCache{cc: redisClient}
	return rc
}

type redisCache struct {
	cc  *redis.Client
	dur time.Duration
}

func (rc *redisCache) Set(ctx context.Context, key, value string, dur time.Duration) error {
	return rc.cc.Set(ctx, key, value, dur).Err()
}

func (rc *redisCache) Get(ctx context.Context, key string) (string, error) {
	res, err := rc.cc.Get(ctx, key).Result()
	switch {
	case err == nil:
		return res, nil
	case errors.Is(err, redis.Nil):
		return "", ussdapp.ErrKeyNotFound
	default:
		return "", err
	}
}

func (rc *redisCache) Delete(ctx context.Context, key string) error {
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

func (rc *redisCache) SetUnique(ctx context.Context, key string, value string) (bool, error) {
	res, err := rc.cc.SAdd(ctx, key, value).Result()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

func (rc *redisCache) ExistInSet(ctx context.Context, key, value string) (bool, error) {
	return rc.cc.SIsMember(ctx, key, value).Result()
}

func (rc *redisCache) DeleteSetValue(ctx context.Context, key, value string) error {
	return rc.cc.SRem(ctx, key, value).Err()
}
