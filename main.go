package main

import (
	"time"

	"github.com/Shimmur/logtailer/cache"
	"github.com/kelseyhightower/envconfig"
	director "github.com/relistan/go-director"
	"github.com/relistan/rubberneck"
	log "github.com/sirupsen/logrus"
)

type Config struct {
	Environment    string        `envconfig:"ENVIRONMENT" default:"dev"`
	BasePath       string        `envconfig:"BASE_PATH" default:"/var/log/pods"`
	DiscoInterval  time.Duration `envconfig:"DISCO_INTERVAL" default:"3s"`
	MaxTrackedLogs int           `envconfig:"MAX_TRACKED_LOGS" default:"100"`
	CacheFilePath  string        `envconfig:"CACHE_FILE_PATH" default:"/var/log/logtailer.json"`
}

func main() {
	var config Config
	err := envconfig.Process("log", &config)
	if err != nil {
		log.Fatal(err.Error())
	}
	rubberneck.Print(config)

	// Some deps for injection
	cache := cache.NewCache(config.MaxTrackedLogs, config.CacheFilePath)
	disco := NewDirListDiscoverer(config.BasePath, config.Environment)
	podDiscoveryLooper := director.NewImmediateTimedLooper(director.FOREVER, config.DiscoInterval, make(chan error))

	tracker := NewPodTracker(podDiscoveryLooper, disco, cache)
	go tracker.Run()

	podDiscoveryLooper.Wait()
}
