package cache

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nxadm/tail"
	"github.com/patrickmn/go-cache"
)

// A Cache is a JSON-persisted map that stores seekinfo for all the logfiles we
// are currently tailing. It is used to prevent re-streaming entire existing
// logfiles when the service is restarted.
type Cache struct {
	store     *cache.Cache
	storePath string
}

// NewCache returns a properly configured cache with the initial size provided
// and a fully-qualified path for file storage.
func NewCache(size int, storePath string) *Cache {
	// Create cache with no expiration and no cleanup interval
	// The size parameter is used as a hint for the initial map capacity
	return &Cache{
		store:     cache.New(cache.NoExpiration, 0),
		storePath: storePath,
	}
}

func (c *Cache) Add(key string, seekInfo *tail.SeekInfo) {
	c.store.Set(key, seekInfo, cache.NoExpiration)
}

func (c *Cache) Get(key string) *tail.SeekInfo {
	if x, found := c.store.Get(key); found {
		return x.(*tail.SeekInfo)
	}
	return nil
}

func (c *Cache) Del(key string) {
	c.store.Delete(key)
}

// Load reads the cache from the file back into memory
func (c *Cache) Load() error {
	data, err := os.ReadFile(c.storePath)
	if err != nil {
		return fmt.Errorf("failed to load cache from %s: %s", c.storePath, err)
	}

	var tempStore map[string]*tail.SeekInfo
	err = json.Unmarshal(data, &tempStore)
	if err != nil {
		return fmt.Errorf("failed to unmarshal cache from %s: %s", c.storePath, err)
	}

	// Load all items into the cache
	for key, seekInfo := range tempStore {
		c.store.Set(key, seekInfo, cache.NoExpiration)
	}

	return nil
}

// Persist stores the cache out to a file
func (c *Cache) Persist() error {
	// Get all items from the cache
	items := c.store.Items()

	// Convert to our expected format
	tempStore := make(map[string]*tail.SeekInfo)
	for key, item := range items {
		tempStore[key] = item.Object.(*tail.SeekInfo)
	}

	data, err := json.Marshal(tempStore)
	if err != nil {
		return fmt.Errorf("failed to persist cache to %s: %s", c.storePath, err)
	}

	err = os.WriteFile(c.storePath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to marshal cache to %s: %s", c.storePath, err)
	}

	return nil
}
