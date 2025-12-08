package database

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/steemdb/web/pkg/utils"
)

// Redis represents Redis connection and operations
type Redis struct {
	client *redis.Client
	config utils.RedisConfig
	logger utils.Logger
}

// parseRedisURI parses Redis URI and extracts connection parameters
// Supports formats:
//   - redis://[password@]host:port/db
//   - redis://host:port
//   - host:port (simple format for backward compatibility)
func parseRedisURI(uri string) (addr, password string, db int, err error) {
	// Handle simple format: host:port
	if !strings.Contains(uri, "://") {
		parts := strings.Split(uri, ":")
		if len(parts) != 2 {
			return "", "", 0, fmt.Errorf("invalid Redis URI format: %s", uri)
		}
		return uri, "", 0, nil
	}

	// Parse full URI format
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to parse Redis URI: %w", err)
	}

	// Extract address
	addr = parsedURL.Host
	if addr == "" {
		return "", "", 0, fmt.Errorf("missing host in Redis URI: %s", uri)
	}

	// Extract password from user info
	if parsedURL.User != nil {
		password, _ = parsedURL.User.Password()
	}

	// Extract database number from path
	db = 0
	if parsedURL.Path != "" && parsedURL.Path != "/" {
		dbStr := strings.TrimPrefix(parsedURL.Path, "/")
		if dbStr != "" {
			db, err = strconv.Atoi(dbStr)
			if err != nil {
				return "", "", 0, fmt.Errorf("invalid database number in Redis URI: %s", dbStr)
			}
		}
	}

	return addr, password, db, nil
}

// NewRedis creates a new Redis connection
func NewRedis(config utils.RedisConfig, logger utils.Logger) (*Redis, error) {
	// Parse Redis URI
	addr, passwordFromURI, dbFromURI, err := parseRedisURI(config.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URI: %w", err)
	}

	// Use password from URI if provided, otherwise use config password
	password := passwordFromURI
	if password == "" {
		password = config.Password
	}

	// Use database from URI if provided, otherwise use config DB
	db := dbFromURI
	if db == 0 && config.DB != 0 {
		db = config.DB
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     config.PoolSize,
		DialTimeout:  config.DialTimeout,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Connected to Redis",
		utils.String("uri", config.URI),
		utils.String("addr", addr),
		utils.Int("db", db))

	return &Redis{
		client: rdb,
		config: config,
		logger: logger,
	}, nil
}

// Close closes the Redis connection
func (r *Redis) Close() error {
	return r.client.Close()
}

// Ping checks if the connection is alive
func (r *Redis) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Client returns the Redis client
func (r *Redis) Client() *redis.Client {
	return r.client
}

// Set sets a key-value pair with expiration
func (r *Redis) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return r.client.Set(ctx, key, value, expiration).Err()
}

// Get gets a value by key
func (r *Redis) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

// Del deletes keys
func (r *Redis) Del(ctx context.Context, keys ...string) error {
	return r.client.Del(ctx, keys...).Err()
}

// Exists checks if keys exist
func (r *Redis) Exists(ctx context.Context, keys ...string) (int64, error) {
	return r.client.Exists(ctx, keys...).Result()
}

// Expire sets expiration for a key
func (r *Redis) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return r.client.Expire(ctx, key, expiration).Err()
}

// HSet sets hash field
func (r *Redis) HSet(ctx context.Context, key string, values ...interface{}) error {
	return r.client.HSet(ctx, key, values...).Err()
}

// HGet gets hash field value
func (r *Redis) HGet(ctx context.Context, key, field string) (string, error) {
	return r.client.HGet(ctx, key, field).Result()
}

// HGetAll gets all hash fields
func (r *Redis) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.client.HGetAll(ctx, key).Result()
}

// HDel deletes hash fields
func (r *Redis) HDel(ctx context.Context, key string, fields ...string) error {
	return r.client.HDel(ctx, key, fields...).Err()
}

// LPush pushes elements to the head of list
func (r *Redis) LPush(ctx context.Context, key string, values ...interface{}) error {
	return r.client.LPush(ctx, key, values...).Err()
}

// RPush pushes elements to the tail of list
func (r *Redis) RPush(ctx context.Context, key string, values ...interface{}) error {
	return r.client.RPush(ctx, key, values...).Err()
}

// LPop pops element from the head of list
func (r *Redis) LPop(ctx context.Context, key string) (string, error) {
	return r.client.LPop(ctx, key).Result()
}

// RPop pops element from the tail of list
func (r *Redis) RPop(ctx context.Context, key string) (string, error) {
	return r.client.RPop(ctx, key).Result()
}

// LRange gets list elements in range
func (r *Redis) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.LRange(ctx, key, start, stop).Result()
}

// LLen gets list length
func (r *Redis) LLen(ctx context.Context, key string) (int64, error) {
	return r.client.LLen(ctx, key).Result()
}

// SAdd adds members to set
func (r *Redis) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return r.client.SAdd(ctx, key, members...).Err()
}

// SMembers gets all set members
func (r *Redis) SMembers(ctx context.Context, key string) ([]string, error) {
	return r.client.SMembers(ctx, key).Result()
}

// SIsMember checks if member exists in set
func (r *Redis) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	return r.client.SIsMember(ctx, key, member).Result()
}

// SRem removes members from set
func (r *Redis) SRem(ctx context.Context, key string, members ...interface{}) error {
	return r.client.SRem(ctx, key, members...).Err()
}

// ZAdd adds members to sorted set
func (r *Redis) ZAdd(ctx context.Context, key string, members ...*redis.Z) error {
	return r.client.ZAdd(ctx, key, members...).Err()
}

// ZRange gets sorted set members in range
func (r *Redis) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.ZRange(ctx, key, start, stop).Result()
}

// ZRevRange gets sorted set members in reverse range
func (r *Redis) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.ZRevRange(ctx, key, start, stop).Result()
}

// ZRangeWithScores gets sorted set members with scores in range
func (r *Redis) ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	return r.client.ZRangeWithScores(ctx, key, start, stop).Result()
}

// ZRevRangeWithScores gets sorted set members with scores in reverse range
func (r *Redis) ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	return r.client.ZRevRangeWithScores(ctx, key, start, stop).Result()
}

// ZRem removes members from sorted set
func (r *Redis) ZRem(ctx context.Context, key string, members ...interface{}) error {
	return r.client.ZRem(ctx, key, members...).Err()
}

// Publish publishes message to channel
func (r *Redis) Publish(ctx context.Context, channel string, message interface{}) error {
	return r.client.Publish(ctx, channel, message).Err()
}

// Subscribe subscribes to channels
func (r *Redis) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return r.client.Subscribe(ctx, channels...)
}

// FlushDB flushes current database
func (r *Redis) FlushDB(ctx context.Context) error {
	return r.client.FlushDB(ctx).Err()
}

// Keys gets keys matching pattern
func (r *Redis) Keys(ctx context.Context, pattern string) ([]string, error) {
	return r.client.Keys(ctx, pattern).Result()
}

// TTL gets key time to live
func (r *Redis) TTL(ctx context.Context, key string) (time.Duration, error) {
	return r.client.TTL(ctx, key).Result()
}
