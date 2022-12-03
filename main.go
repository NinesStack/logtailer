package main

import (
	"errors"
	"os"
	"time"

	"github.com/Shimmur/logtailer/cache"
	"github.com/kelseyhightower/envconfig"
	director "github.com/relistan/go-director"
	"github.com/relistan/rubberneck"
	log "github.com/sirupsen/logrus"
)

type Config struct {
	Environment        string        `envconfig:"ENVIRONMENT" default:"dev"`
	BasePath           string        `envconfig:"BASE_PATH" default:"/var/log/pods"`
	DiscoInterval      time.Duration `envconfig:"DISCO_INTERVAL" default:"3s"`
	MaxTrackedLogs     int           `envconfig:"MAX_TRACKED_LOGS" default:"100"`
	CacheFilePath      string        `envconfig:"CACHE_FILE_PATH" default:"/var/log/logtailer.json"`
	CacheFlushInterval time.Duration `envconfig:"CACHE_FLUSH_INTERVAL" default:"3s"`
	SyslogAddress      string        `envconfig:"SYSLOG_ADDRESS" default:"127.0.0.1:514"`
}

func configureCache(config *Config) *cache.Cache {
	cache := cache.NewCache(config.MaxTrackedLogs, config.CacheFilePath)

	// If the cache file doesn't exist, don't load it
	if _, err := os.Stat(config.CacheFilePath); errors.Is(err, os.ErrNotExist) {
		return cache
	}

	// It existed, we need to load it up
	cache.Load()

	return cache
}

func main() {
	var config Config
	err := envconfig.Process("log", &config)
	if err != nil {
		log.Fatal(err.Error())
	}
	rubberneck.Print(config)

	// Some deps for injection
	cache := configureCache(&config)
	disco := NewDirListDiscoverer(config.BasePath, config.Environment)
	podDiscoveryLooper := director.NewImmediateTimedLooper(
		director.FOREVER, config.DiscoInterval, make(chan error))
	cacheLooper := director.NewTimedLooper(
		director.FOREVER, config.CacheFlushInterval, make(chan error))

	hostname, _ := os.Hostname()

	// Set up and run the tracker
	tracker := NewPodTracker(podDiscoveryLooper, disco, cache, hostname, config.SyslogAddress)
	go tracker.Run()

	// Persist the cache on a timer
	go cacheLooper.Loop(func() error {
		// Get the latest offsets into the main cache
		tracker.FlushOffsets()

		// Write them out
		err := cache.Persist()
		if err != nil {
			log.Errorf("Persisting offsets failed: %s", err)
		}
		return nil
	})

	// Block on the discovery looper for our lifetime
	podDiscoveryLooper.Wait()
}
