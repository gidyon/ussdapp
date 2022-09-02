package ussdapp

import (
	"context"
)

// Cacher represent a generic interface for a cache store
type Cacher interface {
	Set(ctx context.Context, key, value string) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key, value string) error
	SetMap(ctx context.Context, key string, fields map[string]interface{}) error
	GetMap(ctx context.Context, key string) (map[string]string, error)
	DeleteMap(ctx context.Context, key string) error
	SetMapField(ctx context.Context, key string, values ...interface{}) error
	GetMapField(ctx context.Context, key, field string) (string, error)
	DeleteMapField(ctx context.Context, key string, fields ...string) error
}
