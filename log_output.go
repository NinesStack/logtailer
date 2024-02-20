package main

import (
	"context"
	"io/ioutil"
	"strings"
	"time"

	"github.com/Nitro/sidecar-executor/loghooks"
	"github.com/Shimmur/logtailer/reporter"
	limiter "github.com/sethvargo/go-limiter"
	"github.com/sethvargo/go-limiter/memorystore"
	log "github.com/sirupsen/logrus"
)

type LogOutput interface {
	Log(line string)
	Stop()
}

type UDPSyslogger struct {
	syslogger *log.Entry
}

func NewUDPSyslogger(labels map[string]string, address string) *UDPSyslogger {
	syslogger := log.New()

	// We relay UDP syslog because we don't plan to ship it off the box and
	// because it's simplest since there is no backpressure issue to deal with.
	hook, err := loghooks.NewUDPHook(address)
	if err != nil {
		log.Errorf("Error adding hook: %s", err)
	}

	syslogger.Hooks.Add(hook)
	syslogger.SetFormatter(&log.JSONFormatter{
		FieldMap: log.FieldMap{
			log.FieldKeyTime:  "Timestamp",
			log.FieldKeyLevel: "Level",
			log.FieldKeyMsg:   "Payload",
			log.FieldKeyFunc:  "Func",
		},
	})
	syslogger.SetOutput(ioutil.Discard)

	// Add one to the labels length to account for hostname
	fields := make(log.Fields, len(labels)+1)

	// Loop through the fields we're supposed to pass, and add them
	for field, val := range labels {
		fields[field] = val
	}

	return &UDPSyslogger{
		syslogger: syslogger.WithFields(fields),
	}
}

// relayLogs will watch a container and send the logs to Syslog
func (sysl *UDPSyslogger) Log(line string) {
	// 2022-12-03T16:09:51.741778906Z stdout F

	// Wasn't a K8s log line!
	if len(line) < 41 {
		return
	}

	k8sFields := strings.Split(line[0:40], " ")
	descriptor := k8sFields[1]

	// Strip the K8s logging stuff from the log. Because the timestamp length
	// changes sometimes, we check this. It's cheaper than a split on the full
	// log line.
	if line[39] == ' ' {
		line = line[40:len(line)]
	} else {
		line = line[39:len(line)]
	}

	// Attempt to detect errors to log (a la sidecar-executor)
	if descriptor == "stderr" || strings.Contains(strings.ToLower(line), "error") {
		sysl.syslogger.Error(line)
		return
	}

	sysl.syslogger.Info(line)
}

// Stop would clean up any resources if we needed to manage any
func (sysl *UDPSyslogger) Stop() { /* noop */ }

// A RateLimitingLogger is a LogOutput that wraps another LogOutput, adding rate limiting
// capability
type RateLimitingLogger struct {
	limitStore    limiter.Store
	limitReporter *reporter.LimitExceededReporter
	output        LogOutput
	limitKey      string
}

func NewRateLimitingLogger(
	limitReporter *reporter.LimitExceededReporter, tokenLimit int,
	reportInterval time.Duration, key string, output LogOutput) *RateLimitingLogger {

	// Set up the rate limiter
	store, err := memorystore.New(&memorystore.Config{
		// Number of tokens allowed per interval.
		Tokens: uint64(tokenLimit),

		// Interval until tokens reset.
		Interval: reportInterval,
	})

	if err != nil {
		log.Errorf("Unable to create memory store: %s", err)
	}

	return &RateLimitingLogger{
		limitStore:    store,
		limitReporter: limitReporter,
		output:        output,
		limitKey:      key,
	}
}

// isRateLimited compares the tracking key to the stored limit and returns
// a boolen value for whether or not it is limited.
func (logger *RateLimitingLogger) isRateLimited() bool {
	// See if we're going to rate limit this
	limit, remaining, reset, ok, err := logger.limitStore.Take(context.Background(), logger.limitKey)
	log.Debugf("Checking rate limit: %d %d %d %t", limit, remaining, reset, ok)
	if err != nil {
		log.Warnf("Unable to fetch rate limit for %v", logger.limitKey)
		return true // Rate limit it since we can't track
	}

	return !ok
}

// Log is a pass-through to the downstream LogOutput, but checks rate limiting status
func (logger *RateLimitingLogger) Log(line string) {
	if !logger.isRateLimited() {
		logger.output.Log(line)
		return
	}

	logger.limitReporter.Incr()
}

// Stop cleans up our resources on shutdown
func (logger *RateLimitingLogger) Stop() {
	logger.limitStore.Close(context.Background())
}
