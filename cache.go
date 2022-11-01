package ussdapp

import (
	"context"
	"errors"
	"time"
)

var (
	ErrKeyNotFound   = errors.New("key not found")
	ErrValueNotFound = errors.New("value not found")
)

// Cacher represent a generic interface for a cache store
type Cacher interface {
	// Set sets a key value pair that expires after given duration
	//
	// Zero expiration means the key has no expiration time.
	Set(ctx context.Context, key, value string, dur time.Duration) error

	// Get a key value. Users should return ErrKeyNotFound when the key is not found
	Get(ctx context.Context, key string) (string, error)

	// Delete removes the specified key
	Delete(ctx context.Context, key string) error

	// SetMap sets the map fields with the given key
	SetMap(ctx context.Context, key string, fields map[string]interface{}) error

	// GetMap retrieves map values for the specifed key
	GetMap(ctx context.Context, key string) (map[string]string, error)

	// DeleteMap removes map key and values
	DeleteMap(ctx context.Context, key string) error

	// SetMapField will set map field value. Clients should allow the following format when setting map fields values
	//
	//   - SetMapField("myhash", "key1", "value1", "key2", "value2")
	//   - SetMapField("myhash", []string{"key1", "value1", "key2", "value2"})
	//   - SetMapField("myhash", map[string]interface{}{"key1": "value1", "key2": "value2"})
	SetMapField(ctx context.Context, key string, values ...interface{}) error

	// GetMapField retrieves field value for a map
	GetMapField(ctx context.Context, key, field string) (string, error)

	// DeleteMapField removes map field
	DeleteMapField(ctx context.Context, key string, fields ...string) error

	// SetUnique sets a unique value to a set.
	//
	// If the value does exist, should return 0
	// else should return 1 to indicate that the value did not exist in the set
	SetUnique(ctx context.Context, key string, value string) (bool, error)

	// ExistInSet checks if a value exists in the set
	ExistInSet(ctx context.Context, key string, value string) (bool, error)

	// DeleteSetValue removes set value
	DeleteSetValue(ctx context.Context, key string, value string) error
}
