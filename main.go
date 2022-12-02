package main

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/nxadm/tail"
	director "github.com/relistan/go-director"
	"github.com/relistan/rubberneck"
	log "github.com/sirupsen/logrus"
)

type Config struct {
	Environment   string        `envconfig:"ENVIRONMENT" default:"dev"`
	BasePath      string        `envconfig:"BASE_PATH" default:"/var/log/pods"`
	DiscoInterval time.Duration `envconfig:"DISCO_INTERVAL" default:"3s"`
}

type PodTracker struct {
	Pods     map[string]*Pod
	LogTails map[string][]*tail.Tail

	disco  Discoverer
	looper director.Looper
}

func NewPodTracker(looper director.Looper, disco Discoverer) *PodTracker {
	return &PodTracker{
		Pods:     make(map[string]*Pod, 5),
		LogTails: make(map[string][]*tail.Tail, 5),
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

		newPods := make(map[string]*Pod, len(t.Pods))

		for _, pod := range discovered {
			// Handle newly discovered pods
			if _, ok := t.Pods[pod.Name]; !ok {
				log.Infof("new pod --> %s:%s  [%s]", pod.Namespace, pod.ServiceName, pod.Name)

				logFiles, err := t.disco.LogFiles(pod.Name)
				if err != nil {
					log.Warnf("Failed to get logs for pod %s: %s", pod.Name, err)
					continue
				}

				newPods[pod.Name] = pod
				err = t.tailLogs(pod.Name, logFiles)
				if err != nil {
					log.Warnf("Failed to tail logs for pod %s: %s", pod.Name, err)
					continue
				}
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

func (t *PodTracker) tailLogs(podName string, logFiles []string) error {
	for _, filename := range logFiles {
		tailed, err := tail.TailFile(filename, tail.Config{ReOpen: true, Follow: true})
		if err != nil {
			return fmt.Errorf("failed to tail log for %s: %w", podName, err)
		}

		log.Infof("  Adding tail on %s for pod %s", filename, podName)
		t.LogTails[podName] = append(t.LogTails[podName], tailed)
	}

	return nil
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
