package main

import (
	"bytes"
	"errors"
	"os"
	"testing"

	director "github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
	. "github.com/smartystreets/goconvey/convey"
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

func Test_NewPodTracker(t *testing.T) {
	Convey("NewPodTracker()", t, func() {
		looper := director.NewFreeLooper(director.ONCE, make(chan error))
		disco := newMockDisco()

		tracker := NewPodTracker(looper, disco)

		So(tracker.looper, ShouldEqual, looper)
		So(tracker.disco, ShouldEqual, disco)
	})
}
