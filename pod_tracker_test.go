package main

import (
	"errors"
	"testing"

	director "github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
)

type mockDisco struct {
	DiscoverShouldError bool
	LogFilesShouldError bool

	Pods []*Pod
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
	return nil, nil
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

func Test_Run(t *testing.T) {
	Convey("Run()", t, func() {
		looper := director.NewFreeLooper(director.ONCE, make(chan error))
		disco := newMockDisco()

		tracker := NewPodTracker(looper, disco)

		Convey("handles errors from Discover()", func() {
			capture := LogCapture(func() {
				disco.DiscoverShouldError = true
				go tracker.Run()
				err := looper.Wait()

				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "intentional test error")
			})

			So(capture, ShouldContainSubstring, "intentional test error")
		})

		Convey("handles errors from LogFiles()", func() {
			capture := LogCapture(func() {
				disco.LogFilesShouldError = true
				disco.Pods = []*Pod{
					&Pod{Name: "test-pod"},
				}
				go tracker.Run()
				err := looper.Wait()
				So(err, ShouldBeNil)
			})

			So(capture, ShouldContainSubstring, "intentional test error")
		})
	})
}
