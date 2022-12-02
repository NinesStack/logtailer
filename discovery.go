package main

import (
	//"github.com/nxadm/tail"

	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

type Pod struct {
	Name        string
	Namespace   string
	ServiceName string
	Environment string
	Logs        []string
}

type Discoverer interface {
	Discover() ([]*Pod, error)
	LogFiles(pod string) ([]string, error)
}

type DirListDiscoverer struct {
	Dir         string
	Environment string
}

func NewDirListDiscoverer(path, environment string) *DirListDiscoverer {
	return &DirListDiscoverer{
		Dir:         path,
		Environment: environment,
	}
}

func (d *DirListDiscoverer) Discover() ([]*Pod, error) {
	discovered, err := dirList(d.Dir)
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}

	var pods []*Pod
	for _, entry := range discovered {
		namespace, serviceName, err := d.namesFor(entry)
		if err != nil {
			log.Errorf("Error parsing pod directory name: %s", err)
			continue
		}

		pods = append(pods, &Pod{
			Name:        entry,
			Namespace:   namespace,
			ServiceName: serviceName,
			Environment: d.Environment,
		})
	}
	return pods, nil
}

// LogFiles retrievs all the current logs for the pod requested
func (d *DirListDiscoverer) LogFiles(podName string) ([]string, error) {
	baseDir := fmt.Sprintf("%s/%s", d.Dir, podName)

	var logs []string

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("Failed to find logs for %s: %w", podName, err)
		}

		if info == nil {
			return fmt.Errorf("Failed to find logs for %s: %w", podName, err)
		}

		// All the *current* log files are named "0.log"
		if !info.IsDir() && info.Name() == "0.log" {
			logs = append(logs, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return logs, nil
}

// namesFor parses apart the pod filename and gets the pod name and namespace
func (d *DirListDiscoverer) namesFor(podName string) (string, string, error) {
	fields := strings.Split(podName, "-")
	if len(fields) < 7 {
		return "", "", fmt.Errorf("failed to parse podName: %s", podName)
	}

	nameFields := strings.Split(fields[0], "_")
	if len(nameFields) < 2 {
		return "", "", fmt.Errorf("failed to parse podName: %s", podName)
	}

	namespace := nameFields[0]

	// No dashes in the ServiceName
	if len(fields) == 7 {
		return namespace, nameFields[1], nil
	}

	// Had dashes in the ServiceName
	serviceNameParts := []string{nameFields[1]}
	for i := 1; i <= len(fields)-7; i++ {
		serviceNameParts = append(serviceNameParts, fields[i])
	}

	// Underscores are not legal in K8s pod names

	return namespace, strings.Join(serviceNameParts, "-"), nil
}

func dirList(dir string) ([]string, error) {
	var foundFiles []string
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read dir %s: %w", dir, err)
	}

	for _, file := range files {
		foundFiles = append(foundFiles, file.Name())
	}

	return foundFiles, nil
}
