package main

import (
	//"github.com/nxadm/tail"

	"time"

	director "github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
)

type PodTracker struct {
	Pods map[string]*Pod

	disco  Discoverer
	looper director.Looper
}

func NewPodTracker(looper director.Looper, disco Discoverer) *PodTracker {
	return &PodTracker{
		Pods:   make(map[string]*Pod, 5),
		looper: looper,
		disco:  disco,
	}
}

func (t *PodTracker) Run() {
	t.looper.Loop(func() error {
		discovered, err := t.disco.Discover()
		if err != nil {
			log.Error(err.Error())
			return err
		}

		newPods := make(map[string]*Pod, len(t.Pods))

		for _, pod := range discovered {
			if _, ok := t.Pods[pod.Name]; !ok {
				log.Infof("new pod --> %s:%s", pod.Namespace, pod.ServiceName)

				newPods[pod.Name] = pod
				continue
			}

			// Copy it over because we still see this pod
			newPods[pod.Name] = t.Pods[pod.Name]

			// Remove from the old list
			delete(t.Pods, pod.Name)
		}

		// These Pods were no longer present
		for podName, _ := range t.Pods {
			println("drop pod: " + podName)
			// do some pod dropping
		}

		t.Pods = newPods

		return nil
	})
}

func main() {
	disco := NewDirListDiscoverer("Pods", "dev")
	podDiscoveryLooper := director.NewImmediateTimedLooper(director.FOREVER, 3*time.Second, make(chan error))
	tracker := NewPodTracker(podDiscoveryLooper, disco)
	go tracker.Run()
	podDiscoveryLooper.Wait()
}
