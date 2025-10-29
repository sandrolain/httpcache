// Package mongodb provides a MongoDB interface for http caching.
package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/sandrolain/httpcache"
)

// Config holds the configuration for creating a MongoDB cache.
type Config struct {
	// URI is the MongoDB connection URI (e.g., "mongodb://localhost:27017").
	// Required field.
	URI string

	// Database is the name of the database to use for caching.
	// Required field.
	Database string

	// Collection is the name of the collection to use for caching.
	// Optional - defaults to "httpcache".
	Collection string

	// KeyPrefix is a prefix to add to all cache keys.
	// Optional - defaults to "cache:".
	KeyPrefix string

	// Timeout is the timeout for database operations.
	// Optional - defaults to 5 seconds.
	Timeout time.Duration

	// TTL is the time-to-live for cache entries.
	// Optional - if set, creates a TTL index on the createdAt field.
	TTL time.Duration

	// ClientOptions are additional options to pass to mongo.Connect.
	// Optional.
	ClientOptions *options.ClientOptions
}

// cacheEntry represents a cache entry in MongoDB.
type cacheEntry struct {
	Key       string    `bson:"_id"`
	Data      []byte    `bson:"data"`
	CreatedAt time.Time `bson:"createdAt"`
}

// cache is an implementation of httpcache.Cache that caches responses in MongoDB.
type cache struct {
	client     *mongo.Client
	collection *mongo.Collection
	keyPrefix  string
	timeout    time.Duration
}

// cacheKey adds the configured prefix to the key.
func (c cache) cacheKey(key string) string {
	return c.keyPrefix + key
}

// Get returns the response corresponding to key if present.
func (c cache) Get(key string) (resp []byte, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	var entry cacheEntry
	err := c.collection.FindOne(ctx, bson.M{"_id": c.cacheKey(key)}).Decode(&entry)
	if err != nil {
		return nil, false
	}

	return entry.Data, true
}

// Set saves a response to the cache as key.
func (c cache) Set(key string, resp []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	entry := cacheEntry{
		Key:       c.cacheKey(key),
		Data:      resp,
		CreatedAt: time.Now(),
	}

	opts := options.Replace().SetUpsert(true)
	_, err := c.collection.ReplaceOne(ctx, bson.M{"_id": entry.Key}, entry, opts)
	if err != nil {
		httpcache.GetLogger().Warn("failed to write to MongoDB cache", "key", key, "error", err)
	}
}

// Delete removes the response with key from the cache.
func (c cache) Delete(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	_, err := c.collection.DeleteOne(ctx, bson.M{"_id": c.cacheKey(key)})
	if err != nil {
		httpcache.GetLogger().Warn("failed to delete from MongoDB cache", "key", key, "error", err)
	}
}

// Close disconnects from MongoDB.
// This method should be called when done to properly clean up resources.
func (c cache) Close() error {
	if c.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		defer cancel()
		return c.client.Disconnect(ctx)
	}
	return nil
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Collection: "httpcache",
		KeyPrefix:  "cache:",
		Timeout:    5 * time.Second,
	}
}

// New creates a new Cache with the given configuration.
// It establishes a connection to MongoDB and creates the necessary indexes.
// The caller should call Close() on the returned cache when done to clean up resources.
func New(ctx context.Context, config Config) (httpcache.Cache, error) {
	if config.URI == "" {
		return nil, fmt.Errorf("MongoDB URI is required")
	}
	if config.Database == "" {
		return nil, fmt.Errorf("database name is required")
	}

	// Apply defaults for zero values
	if config.Collection == "" {
		config.Collection = DefaultConfig().Collection
	}
	if config.KeyPrefix == "" {
		config.KeyPrefix = DefaultConfig().KeyPrefix
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultConfig().Timeout
	}

	// Create client options
	clientOpts := options.Client().ApplyURI(config.URI)
	if config.ClientOptions != nil {
		clientOpts = config.ClientOptions.ApplyURI(config.URI)
	}

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping to verify connection
	pingCtx, pingCancel := context.WithTimeout(ctx, config.Timeout)
	defer pingCancel()

	if err := client.Ping(pingCtx, nil); err != nil {
		if disconnectErr := client.Disconnect(ctx); disconnectErr != nil {
			httpcache.GetLogger().Warn("failed to disconnect client after ping error", "error", disconnectErr)
		}
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	collection := client.Database(config.Database).Collection(config.Collection)

	c := cache{
		client:     client,
		collection: collection,
		keyPrefix:  config.KeyPrefix,
		timeout:    config.Timeout,
	}

	// Create TTL index if TTL is configured
	if config.TTL > 0 {
		if err := c.createTTLIndex(ctx, config.TTL); err != nil {
			if disconnectErr := client.Disconnect(ctx); disconnectErr != nil {
				httpcache.GetLogger().Warn("failed to disconnect client after TTL index error", "error", disconnectErr)
			}
			return nil, fmt.Errorf("failed to create TTL index: %w", err)
		}
	}

	return c, nil
}

// NewWithClient returns a new Cache with the given MongoDB client.
// This constructor is useful when you want to manage the MongoDB connection yourself.
// The returned cache will not close the MongoDB client when Close() is called.
func NewWithClient(client *mongo.Client, database, collection string, config Config) (httpcache.Cache, error) {
	if client == nil {
		return nil, fmt.Errorf("MongoDB client is required")
	}
	if database == "" {
		return nil, fmt.Errorf("database name is required")
	}

	// Apply defaults
	if collection == "" {
		collection = DefaultConfig().Collection
	}
	if config.KeyPrefix == "" {
		config.KeyPrefix = DefaultConfig().KeyPrefix
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultConfig().Timeout
	}

	return cache{
		client:     nil, // Don't store client to prevent closing it
		collection: client.Database(database).Collection(collection),
		keyPrefix:  config.KeyPrefix,
		timeout:    config.Timeout,
	}, nil
}

// createTTLIndex creates a TTL index on the createdAt field.
func (c cache) createTTLIndex(ctx context.Context, ttl time.Duration) error {
	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "createdAt", Value: 1}},
		Options: options.Index().
			SetExpireAfterSeconds(int32(ttl.Seconds())).
			SetName("httpcache_ttl"),
	}

	indexCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	_, err := c.collection.Indexes().CreateOne(indexCtx, indexModel)
	return err
}
