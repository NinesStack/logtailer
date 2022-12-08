package main

import (
	"errors"
	"os"
	"time"

	"github.com/Shimmur/logtailer/cache"
	"github.com/Shimmur/logtailer/reporter"
	"github.com/kelseyhightower/envconfig"
	director "github.com/relistan/go-director"
	"github.com/relistan/rubberneck"
	log "github.com/sirupsen/logrus"
)

const (
	// NewRelicBaseURL is the base URL where we'll send events
	NewRelicBaseURL = "https://insights-collector.newrelic.com/v1/accounts/"
)

type Config struct {
	Environment    string        `envconfig:"ENVIRONMENT" default:"dev"`
	BasePath       string        `envconfig:"BASE_PATH" default:"/var/log/pods"`
	DiscoInterval  time.Duration `envconfig:"DISCO_INTERVAL" default:"5s"`
	MaxTrackedLogs int           `envconfig:"MAX_TRACKED_LOGS" default:"100"`

	CacheFilePath      string        `envconfig:"CACHE_FILE_PATH" default:"/var/log/logtailer.json"`
	CacheFlushInterval time.Duration `envconfig:"CACHE_FLUSH_INTERVAL" default:"3s"`

	SyslogAddress string `envconfig:"SYSLOG_ADDRESS" default:"127.0.0.1:514"`

	NewRelicAccount string `envconfig:"NEW_RELIC_ACCOUNT"`
	NewRelicKey     string `envconfig:"NEW_RELIC_LICENSE_KEY"`

	TokenLimit    int           `envconfig:"TOKEN_LIMIT" default:"300"`
	LimitInterval time.Duration `envconfig:"LIMIT_INTERVAL" default:"1m"`

	KubeHost      string        `envconfig:"KUBERNETES_SERVICE_HOST" default:"127.0.0.1"`
	KubePort      int           `envconfig:"KUBERNETES_SERVICE_PORT" default:"8080"`
	KubeTimeout   time.Duration `envconfig:"KUBERNETES_TIMEOUT" default:"3s"`
	KubeCredsPath string        `envconfig:"KUBERNETES_CREDS_PATH" default:"/var/run/secrets/kubernetes.io/serviceaccount"`

	Debug bool `envconfig:"DEBUG" default:"false"`
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

// NewTailerWithUDPSyslog is passed to PodTracker to generate new Tailers with
// UDP Syslog output. It uses a closure to pass in cache, address, and hostname.
func NewTailerWithUDPSyslog(c *cache.Cache, hostname string,
	config *Config, rptr *reporter.LimitExceededReporter) NewTailerFunc {

	return func(pod *Pod) LogTailer {
		// Configure the fields we log to Syslog
		udpLogger := NewUDPSyslogger(map[string]string{
			"ServiceName": pod.ServiceName,
			"Environment": pod.Environment,
			"PodName":     pod.Name,
			"Hostname":    hostname,
		}, config.SyslogAddress)

		// Inject the UDPSyslogger into the RateLimitingLogger
		limitingLogger := NewRateLimitingLogger(rptr, config.TokenLimit, config.LimitInterval, "ServiceName", udpLogger)

		// Wrap the return value from NewTailer as an interface
		return NewTailer(pod, c, limitingLogger)
	}
}

// getHostname figures out what the hostname is that we should use for log records
func getHostname() string {
	// This allows us to override the hostname for running inside a container and having the
	// host's hostname in logs
	if hostname := os.Getenv("HOSTNAME"); hostname != "" {
		return hostname
	}

	// Otherwise fall back to the Uname from syscall
	hostname, _ := os.Hostname()
	return hostname
}

func main() {
	var config Config
	err := envconfig.Process("log", &config)
	if err != nil {
		log.Fatal(err.Error())
	}

	// Redact the secret key
	var redacted = "[REDACTED]"
	maskFunc := func(argument string) *string {
		if argument == "NewRelicKey" {
			return &redacted
		}
		return nil
	}
	printer := rubberneck.NewPrinterWithKeyMasking(log.Printf, maskFunc, rubberneck.NoAddLineFeed)
	printer.Print(config)

	// Maybe enable debug logging for this service
	if config.Debug {
		log.SetLevel(log.DebugLevel)
	}

	var filter DiscoveryFilter

	// Some deps for injection
	cache := configureCache(&config)
	podFilter := NewPodFilter(
		config.KubeHost, config.KubePort, config.KubeTimeout, config.KubeCredsPath,
	)
	disco := NewDirListDiscoverer(config.BasePath, config.Environment)
	rptr := reporter.NewLimitExceededReporter(
		NewRelicBaseURL, config.NewRelicKey, config.NewRelicAccount,
	)

	podDiscoveryLooper := director.NewImmediateTimedLooper(
		director.FOREVER, config.DiscoInterval, make(chan error))
	cacheLooper := director.NewTimedLooper(
		director.FOREVER, config.CacheFlushInterval, make(chan error))

	// In the event our filter can't find the right creds, etc, we fail open
	if podFilter != nil {
		filter = podFilter
	} else {
		log.Warn("Failed to configure filter, proceeding anyway using stub...")
		filter = &StubFilter{}
	}

	// Set up and run the tracker
	newTailerFunc := NewTailerWithUDPSyslog(cache, getHostname(), &config, rptr)
	tracker := NewPodTracker(podDiscoveryLooper, disco, newTailerFunc, filter)
	go tracker.Run()

	// Run the reporter
	go rptr.Run()

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
