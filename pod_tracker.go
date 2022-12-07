package main

import (
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
			// Handle newly discovered pods
			if _, ok := t.LogTails[pod.Name]; !ok {
				log.Infof("new pod --> %s:%s  [%s]", pod.Namespace, pod.ServiceName, pod.Name)

				shouldTail, err := t.Filter.ShouldTailLogs(pod)
				if err != nil {
					log.Errorf("Failed to check filter for pod %s, disabling logging", pod.Name)
					continue
				}

				logFiles, err := t.disco.LogFiles(pod.Name)
				if err != nil {
					log.Warnf("Failed to get logs for pod %s: %s", pod.Name, err)
					continue
				}

				// We keep state on these, but empty the list of log files to prevent tailing
				if !shouldTail {
					log.Infof("Skipping pod %s because filter says to", pod.Name)
					logFiles = []string{}
				}

				tailer := t.newTailerFunc(pod)
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
