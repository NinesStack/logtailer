package main

import (
	"encoding/json"
	"net/http"
	"sync"

	director "github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
)

var startup sync.Once

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
			// Handle existing/known pods
			var wasKnown bool
			t.withLock(func() {
				tailer, ok := t.LogTails[pod.Name]

				if ok {
					wasKnown = true // Can't continue from in here

					// Copy it over because we still see this pod
					newTails[pod.Name] = tailer

					// Find all the new files for the pod
					logFiles, err := t.disco.LogFiles(pod.Name)
					if err != nil {
						log.Warnf("Failed to get logs for pod %s: %s", pod.Name, err)
						return
					}

					// Update the followed files
					err = tailer.TailLogs(logFiles)
					if err != nil {
						log.Errorf("Failed to tail logs for %s: %s", pod.Name, err)
						return
					}
					// State for debugging
					pod.Logs = logFiles
				}
			})

			if wasKnown {
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

				pod.Logs = logFiles

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

			if _, ok := tailer.(*MockTailer); !ok {
				log.Infof("Adding and running new tailer for pod %s", pod.Name)
			}

			newTails[pod.Name] = tailer

			// Will exit when the looper is stopped, when Stop() is called on the Tailer
			tailer.Run()
		}

		// Swap the new list with the old list
		var oldTails map[string]LogTailer
		t.withLock(func() {
			oldTails = t.LogTails
			t.LogTails = newTails
		})

		// Iterate over the old list to remove pods no longer present
		t.withReadLock(func() {
			for podName, tailer := range oldTails {
				if _, ok := t.LogTails[podName]; !ok {
					// Do some pod dropping
					log.Infof("drop pod: %s", podName)
					tailer.Stop()
				}
			}
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

func (t *PodTracker) ServeHTTP() {
	go func() {
		// Set up the route and handler.
		http.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
			t.tailsLock.RLock()
			defer t.tailsLock.RUnlock()

			// Set the Content-Type header.
			w.Header().Set("Content-Type", "application/json")

			err := json.NewEncoder(w).Encode(t.LogTails)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})

		// Start the server.
		log.Println("State server starting on :8080...")
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			log.Error(err.Error())
		}
	}()
}
