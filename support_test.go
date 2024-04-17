package main

import (
	"bytes"
	"errors"
	"os"
	"sync"

	log "github.com/sirupsen/logrus"
)

// LogCapture logs for async testing where we can't get a nice handle on thigns
func LogCapture(fn func()) string {
	capture := &bytes.Buffer{}
	log.SetOutput(capture)
	fn()
	log.SetOutput(os.Stdout)

	return capture.String()
}

// mockDisco is a mock that implements the Discoverer interface, for testing
type mockDisco struct {
	DiscoverShouldError bool
	LogFilesShouldError bool

	Pods []*Pod
	Logs []string
}

func newMockDisco() *mockDisco {
	return &mockDisco{}
}

func (d *mockDisco) Discover() ([]*Pod, error) {
	if d.DiscoverShouldError {
		return nil, errors.New("intentional test error")
	}

	if d.Pods != nil {
		return d.Pods, nil
	}

	return []*Pod{}, nil
}

func (d *mockDisco) LogFiles(pod string) ([]string, error) {
	if d.LogFilesShouldError {
		return nil, errors.New("intentional test error")
	}
	return d.Logs, nil
}

// NewMockTailerFunc is injected into a PodTracker get it to use MockTailers
func NewMockTailerFunc(tailer *MockTailer) NewTailerFunc {
	return func(pod *Pod) LogTailer {
		tailer.PodTailed = pod
		return tailer
	}
}

// mockFilter implements the DiscoveryFilter interface
type mockFilter struct {
	ShouldNotTailFor map[string]bool
}

func (m *mockFilter) ShouldTailLogs(pod *Pod) (bool, error) {
	if m.ShouldNotTailFor != nil {
		if _, ok := m.ShouldNotTailFor[pod.Name]; ok {
			return false, nil
		}
	}
	return true, nil
}

// mockLogOutput implements the LogOutput interface
type mockLogOutput struct {
	LastLogged    *LogLine
	WasCalled     bool
	CallCount     int
	StopWasCalled bool
	sync.Mutex
}

func (m *mockLogOutput) Log(line *LogLine) {
	m.Lock()
	m.WasCalled = true
	m.LastLogged = line
	m.CallCount += 1
	m.Unlock()
}

func (m *mockLogOutput) Stop() {
	m.StopWasCalled = true
}
