package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/nxadm/tail"
)

// A Cache is a JSON-persisted map that stores seekinfo for all the logfiles we
// are currently tailing. It is used to prevent re-streaming entire existing
// logfiles when the service is restarted.
type Cache struct {
	lock      sync.RWMutex
	store     map[string]*tail.SeekInfo
	storePath string
}

// NewCache returns a properly configured cache with the initial size provided
// and a fully-qualified path for file storage.
func NewCache(size int, storePath string) *Cache {
	return &Cache{
		store:     make(map[string]*tail.SeekInfo, size),
		storePath: storePath,
	}
}

func (c *Cache) Add(key string, seekInfo *tail.SeekInfo) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.store[key] = seekInfo
}

func (c *Cache) Get(key string) *tail.SeekInfo {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.store[key]
}

func (c *Cache) Del(key string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	delete(c.store, key)
}

// Load reads the cache from the file back into memory
func (c *Cache) Load() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	data, err := os.ReadFile(c.storePath)
	if err != nil {
		return fmt.Errorf("failed to load cache from %s: %s", c.storePath, err)
	}

	err = json.Unmarshal(data, &c.store)
	if err != nil {
		return fmt.Errorf("failed to load cache from %s: %s", c.storePath, err)
	}

	return nil
}

// Persist stores the cache out to a file
func (c *Cache) Persist() error {
	c.lock.RLock()
	defer c.lock.RUnlock()

	data, err := json.Marshal(c.store)
	if err != nil {
		return fmt.Errorf("failed to persist to %s: %s", c.storePath, err)
	}

	err = os.WriteFile(c.storePath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to persist to %s: %s", c.storePath, err)
	}

	return nil
}
