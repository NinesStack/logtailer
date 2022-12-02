package main

import (
	"time"

	"github.com/kelseyhightower/envconfig"
	director "github.com/relistan/go-director"
	"github.com/relistan/rubberneck"
	log "github.com/sirupsen/logrus"
)

type Config struct {
	Environment   string        `envconfig:"ENVIRONMENT" default:"dev"`
	BasePath      string        `envconfig:"BASE_PATH" default:"/var/log/pods"`
	DiscoInterval time.Duration `envconfig:"DISCO_INTERVAL" default:"3s"`
}

func main() {
	var config Config
	err := envconfig.Process("log", &config)
	if err != nil {
		log.Fatal(err.Error())
	}
	rubberneck.Print(config)

	disco := NewDirListDiscoverer(config.BasePath, config.Environment)
	podDiscoveryLooper := director.NewImmediateTimedLooper(director.FOREVER, config.DiscoInterval, make(chan error))

	tracker := NewPodTracker(podDiscoveryLooper, disco)
	go tracker.Run()

	podDiscoveryLooper.Wait()
}
