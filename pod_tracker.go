package main

import (
	"sync"

	director "github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
)

type NewTailerFunc func(pod *Pod) LogTailer

// A PodTracker keeps track of all the Tailers and orchestrates them based on
// what it finds out from discovery, on time loop controlled by the looper.
type PodTracker struct {
	LogTails map[string]LogTailer
	Filter   DiscoveryFilter

	disco         Discoverer
	looper        director.Looper
	newTailerFunc NewTailerFunc

	tailsLock sync.RWMutex
}

// NewPodTracker configures a PodTracker for use, assigning the given Looper
// and Discoverer, and making sure the caching map is made.
func NewPodTracker(looper director.Looper, disco Discoverer,
	newTailerFunc NewTailerFunc, filter DiscoveryFilter) *PodTracker {

	return &PodTracker{
		LogTails:      make(map[string]LogTailer, 5),
		looper:        looper,
		disco:         disco,
		newTailerFunc: newTailerFunc,
		Filter:        filter,
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
			var ok bool
			t.withReadLock(func() { _, ok = t.LogTails[pod.Name] })

			// Handle existing/known pods
			if ok {
				t.withLock(func() {
					// Copy it over because we still see this pod
					newTails[pod.Name] = t.LogTails[pod.Name]

					// Remove from the old list
					delete(t.LogTails, pod.Name)
				})

				continue
			}

			// Handle newly discovered pods
			log.Infof("new pod --> %s:%s  [%s]", pod.Namespace, pod.ServiceName, pod.Name)

			shouldTail, err := t.Filter.ShouldTailLogs(pod)
			if err != nil {
				log.Errorf(
					"Failed to check filter for pod %s, disabling logging: %s", err, pod.Name,
				)
				continue
			}

			var tailer LogTailer

			if shouldTail {
				// Find the files and actually tail them
				logFiles, err := t.disco.LogFiles(pod.Name)
				if err != nil {
					log.Warnf("Failed to get logs for pod %s: %s", pod.Name, err)
					continue
				}

				tailer = t.newTailerFunc(pod)

				err = tailer.TailLogs(logFiles)
				if err != nil {
					log.Warnf("Failed to tail logs for pod %s: %s", pod.Name, err)
					continue
				}

			} else {
				// We want to keep state on these, so we just use a mock instead
				log.Infof("Skipping pod %s because filter says to", pod.Name)
				tailer = &MockTailer{PodTailed: pod}
			}

			log.Infof("Adding and running new tailer for pod %s", pod.Name)
			newTails[pod.Name] = tailer

			// Will exit when the looper is stopped, when Stop() is called on the Tailer
			tailer.Run()
		}

		// These Pods were no longer present
		t.withLock(func() {
			for podName, tailer := range t.LogTails {
				log.Infof("drop pod: %s", podName)

				// Do some pod dropping
				tailer.Stop()
			}

			t.LogTails = newTails
		})

		return nil
	})
}

func (t *PodTracker) FlushOffsets() {
	t.withReadLock(func() {
		for _, tailer := range t.LogTails {
			tailer.FlushOffsets()
		}
	})
}

func (t *PodTracker) withReadLock(fn func()) {
	t.tailsLock.RLock()
	fn()
	t.tailsLock.RUnlock()
}

func (t *PodTracker) withLock(fn func()) {
	t.tailsLock.Lock()
	fn()
	t.tailsLock.Unlock()
}
