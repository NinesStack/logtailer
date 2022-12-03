package main

import (
	"github.com/Shimmur/logtailer/cache"
	director "github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
)

// A PodTracker keeps a cache of all the Tailers and orchestrates them based on
// what it finds out from discovery, on time loop controlled by the looper.
type PodTracker struct {
	LogTails map[string]LogTailer

	disco         Discoverer
	looper        director.Looper
	cache         *cache.Cache
	newTailerFunc func(pod *Pod, cache *cache.Cache) LogTailer
}

// NewPodTracker configures a PodTracker for use, assigning the given Looper
// and Discoverer, and making sure the caching map is made.
func NewPodTracker(looper director.Looper, disco Discoverer, c *cache.Cache, hostname string, address string) *PodTracker {
	return &PodTracker{
		LogTails: make(map[string]LogTailer, 5),
		looper:   looper,
		disco:    disco,
		cache:    c,
		newTailerFunc: func(pod *Pod, c *cache.Cache) LogTailer {
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": pod.ServiceName,
				"Environment": pod.Environment,
				"PodName":     pod.Name,
				"Hostname":    hostname,
			}, address)

			// Wrap the return value from NewTailer as an interface
			return NewTailer(pod, c, logger)
		},
	}
}

// Run invokes the looper to poll discovery and then add or remove Pods from
// tracking. The work of the actual file tailing is done by the Tailers.
func (t *PodTracker) Run() {
	t.looper.Loop(func() error {
		discovered, err := t.disco.Discover()
		if err != nil {
			log.Error(err.Error())
			return err
		}

		newTails := make(map[string]LogTailer, len(t.LogTails))

		for _, pod := range discovered {
			// Handle newly discovered pods
			if _, ok := t.LogTails[pod.Name]; !ok {
				log.Infof("new pod --> %s:%s  [%s]", pod.Namespace, pod.ServiceName, pod.Name)

				logFiles, err := t.disco.LogFiles(pod.Name)
				if err != nil {
					log.Warnf("Failed to get logs for pod %s: %s", pod.Name, err)
					continue
				}

				tailer := t.newTailerFunc(pod, t.cache)
				err = tailer.TailLogs(logFiles)
				if err != nil {
					log.Warnf("Failed to tail logs for pod %s: %s", pod.Name, err)
					continue
				}

				newTails[pod.Name] = tailer

				// Will exit when the looper is stopped, when Stop() is called on the Tailer
				go tailer.Run()

				continue
			}

			// Copy it over because we still see this pod
			newTails[pod.Name] = t.LogTails[pod.Name]

			// Remove from the old list
			delete(t.LogTails, pod.Name)
		}

		// These Pods were no longer present
		for podName, tailer := range t.LogTails {
			log.Infof("drop pod: %s", podName)

			// Do some pod dropping
			tailer.Stop()
		}

		t.LogTails = newTails

		return nil
	})
}

func (t *PodTracker) FlushOffsets() {
	for _, tailer := range t.LogTails {
		tailer.FlushOffsets()
	}
}
