package main

import (
	"testing"

	director "github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
)

// NOTE: Because of the extremely async behavior of this service, the following
// tests rely a lot on the output of logs to make sure that state we can't see
// what handled properly.

func Test_Run(t *testing.T) {
	Convey("Run()", t, func() {
		looper := director.NewFreeLooper(director.ONCE, make(chan error))
		disco := newMockDisco()

		tracker := NewPodTracker(looper, disco)

		Convey("tails the logs for a newly discovered pod", func() {
			So(len(tracker.LogTails), ShouldEqual, 0)

			capture := LogCapture(func() {
				disco.Pods = []*Pod{
					&Pod{Name: "default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"},
				}
				disco.Logs = []string{
					"fixtures/default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499/chopper/0.log",
				}

				go tracker.Run()
				err := looper.Wait()
				So(err, ShouldBeNil)
			})

			So(capture, ShouldContainSubstring, "Adding tail on fixtures/default_chopper-f5b66c6bf")
			So(capture, ShouldNotContainSubstring, "Waiting for") // This happens if the file isn't found
			So(len(tracker.LogTails), ShouldEqual, 1)
		})

		Convey("continues to track a pod that was already seen", func() {
			_ = LogCapture(func() {
				disco.Pods = []*Pod{
					&Pod{Name: "default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"},
				}
				disco.Logs = []string{
					"fixtures/default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499/chopper/0.log",
				}

				go tracker.Run()
				err := looper.Wait()
				So(err, ShouldBeNil)
			})
			So(len(tracker.LogTails), ShouldEqual, 1)
			_, ok := tracker.LogTails["default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"]
			So(ok, ShouldBeTrue)

			capture := LogCapture(func() {
				go tracker.Run()
				err := looper.Wait()
				So(err, ShouldBeNil)
			})

			So(capture, ShouldNotContainSubstring, "Adding tail on fixtures/default_chopper-f5b66c6bf")
			So(len(tracker.LogTails), ShouldEqual, 1)

			_, ok = tracker.LogTails["default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"]
			So(ok, ShouldBeTrue)
		})

		Convey("drops a pod that is no longer present", func() {
			_ = LogCapture(func() {
				disco.Pods = []*Pod{
					&Pod{Name: "default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"},
				}
				disco.Logs = []string{
					"fixtures/default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499/chopper/0.log",
				}

				go tracker.Run()
				err := looper.Wait()
				So(err, ShouldBeNil)
			})
			So(len(tracker.LogTails), ShouldEqual, 1)
			_, ok := tracker.LogTails["default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"]
			So(ok, ShouldBeTrue)

			disco.Pods = []*Pod{}

			capture := LogCapture(func() {
				go tracker.Run()
				err := looper.Wait()
				So(err, ShouldBeNil)
			})

			So(capture, ShouldContainSubstring, "drop pod: default_chopper")
			So(len(tracker.LogTails), ShouldEqual, 0)

			_, ok = tracker.LogTails["default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"]
			So(ok, ShouldBeFalse)
		})

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
