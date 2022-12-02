package main

import (
	director "github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
)

type PodTracker struct {
	LogTails map[string]*Tailer

	disco  Discoverer
	looper director.Looper
}

func NewPodTracker(looper director.Looper, disco Discoverer) *PodTracker {
	return &PodTracker{
		LogTails: make(map[string]*Tailer, 5),
		looper:   looper,
		disco:    disco,
	}
}

func (t *PodTracker) Run() {
	t.looper.Loop(func() error {
		discovered, err := t.disco.Discover()
		if err != nil {
			log.Error(err.Error())
			return err
		}

		newTails := make(map[string]*Tailer, len(t.LogTails))

		for _, pod := range discovered {
			// Handle newly discovered pods
			if _, ok := t.LogTails[pod.Name]; !ok {
				log.Infof("new pod --> %s:%s  [%s]", pod.Namespace, pod.ServiceName, pod.Name)

				logFiles, err := t.disco.LogFiles(pod.Name)
				if err != nil {
					log.Warnf("Failed to get logs for pod %s: %s", pod.Name, err)
					continue
				}

				tailer := NewTailer(pod)
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
