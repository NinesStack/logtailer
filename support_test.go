package main

import (
	"bytes"
	"errors"
	"os"

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

// mockTailer implements the LogTailer interface, for testing
type mockTailer struct {
	FlushOffsetsWasCalled bool
	RunWasCalled          bool
	StopWasCalled         bool

	PodTailed *Pod
}

func (t *mockTailer) TailLogs(logFiles []string) error { return nil }
func (t *mockTailer) Run()                             { t.RunWasCalled = true }
func (t *mockTailer) FlushOffsets()                    { t.FlushOffsetsWasCalled = true }
func (t *mockTailer) Stop()                            { t.StopWasCalled = true }

// NewMockTailerFunc is injected into a PodTracker get it to use mockTailers
func NewMockTailerFunc(tailer *mockTailer) NewTailerFunc {
	return func(pod *Pod) LogTailer {
		tailer.PodTailed = pod
		return tailer
	}
}

// mockFilter implements the DiscoveryFilter interface
type mockFilter struct {}

func (m *mockFilter) ShouldTailLogs(pod *Pod) (bool, error) {
	return true, nil
}
