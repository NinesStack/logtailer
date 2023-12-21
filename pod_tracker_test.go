package main

import (
	"os"
	"testing"
	"time"

	"github.com/Shimmur/logtailer/cache"
	"github.com/Shimmur/logtailer/reporter"
	director "github.com/relistan/go-director"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_NewPodTracker(t *testing.T) {
	Convey("NewPodTracker()", t, func() {
		looper := director.NewFreeLooper(director.ONCE, make(chan error))
		disco := newMockDisco()

		tracker := NewPodTracker(looper, disco, NewMockTailerFunc(&mockTailer{}), &mockFilter{})

		So(tracker.looper, ShouldEqual, looper)
		So(tracker.disco, ShouldEqual, disco)
	})
}

// NOTE: Because of the extremely async behavior of this service, the following
// tests rely a lot on the output of logs to make sure that state we can't see
// what handled properly.

func Test_Run(t *testing.T) {
	Convey("Run()", t, func() {
		cacheFile, err := os.CreateTemp("", "seekInfoCache*")
		So(err, ShouldBeNil)

		cache := cache.NewCache(5, cacheFile.Name())
		looper := director.NewFreeLooper(director.ONCE, make(chan error))
		disco := newMockDisco()

		config := &Config{
			SyslogAddress:     "127.0.0.1",
			TokenLimit:        300,
			LimitInterval:     1 * time.Minute,
			LimitSessionTTL:   1 * time.Hour,
			LimitSessionSweep: 1 * time.Hour,
		}

		rptr := reporter.NewLimitExceededReporter("", "", "")

		tracker := NewPodTracker(looper, disco, NewTailerWithUDPSyslog(cache, "beowulf", config, rptr), &mockFilter{})

		Convey("tails the logs for a newly discovered pod", func() {
			So(len(tracker.LogTails), ShouldEqual, 0)

			capture := LogCapture(func() {
				disco.Pods = []*Pod{
					&Pod{Name: "default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"},
				}
				disco.Logs = []string{
					fixturesDir + "/default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499/chopper/0.log",
				}

				go tracker.Run()
				err := looper.Wait()
				So(err, ShouldBeNil)
			})

			So(capture, ShouldContainSubstring, "Adding tail on fixtures/pods/default_chopper-f5b66c6bf")
			So(capture, ShouldNotContainSubstring, "Waiting for") // This happens if the file isn't found
			So(len(tracker.LogTails), ShouldEqual, 1)
		})

		Convey("does not tail the logs for a pod that is filtered", func() {
			So(len(tracker.LogTails), ShouldEqual, 0)
			filter := &mockFilter{
				ShouldNotTailFor: map[string]bool{
					"default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499": true,
				},
			}
			tracker.Filter = filter

			capture := LogCapture(func() {
				disco.Pods = []*Pod{
					&Pod{Name: "default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"},
				}
				disco.Logs = []string{
					fixturesDir + "/default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499/chopper/0.log",
				}

				go tracker.Run()
				err := looper.Wait()
				So(err, ShouldBeNil)
			})

			So(capture, ShouldContainSubstring, "Skipping pod default_chopper-f5b66c6bf")
			So(capture, ShouldNotContainSubstring, "Adding tail on fixtures/pods/default_chopper-f5b66c6bf")
			So(capture, ShouldNotContainSubstring, "Waiting for") // This happens if the file isn't found
			So(len(tracker.LogTails), ShouldEqual, 1)

			// But, there should not be any logfiles tracked
			activeTails := tracker.LogTails["default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"].(*Tailer)
			So(len(activeTails.LogTails), ShouldEqual, 0)
		})

		Convey("continues to track a pod that was already seen", func() {
			_ = LogCapture(func() {
				disco.Pods = []*Pod{
					&Pod{Name: "default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"},
				}
				disco.Logs = []string{
					fixturesDir + "/default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499/chopper/0.log",
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

			So(capture, ShouldNotContainSubstring, "Adding tail on fixtures/pods/default_chopper-f5b66c6bf")
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
					fixturesDir + "/default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499/chopper/0.log",
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

		Convey("starts the LogTailer properly", func() {
			tailer := &mockTailer{}
			tracker := NewPodTracker(looper, disco, NewMockTailerFunc(tailer), &mockFilter{})
			disco.Pods = []*Pod{
				&Pod{Name: "default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"},
			}

			_ = LogCapture(func() {
				go tracker.Run()
				err := looper.Wait()
				So(err, ShouldBeNil)
			})

			// Janky but have to wait for the first run
			time.Sleep(10 * time.Millisecond)

			So(tailer.RunWasCalled, ShouldBeTrue)
		})
	})
}

func Test_FlushOffsets(t *testing.T) {
	Convey("FlushOffsets()", t, func() {
		looper := director.NewFreeLooper(director.ONCE, make(chan error))
		disco := newMockDisco()

		Convey("flushes logs for all tailers", func() {
			disco.Pods = []*Pod{
				&Pod{Name: "default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499"},
			}
			disco.Logs = []string{
				fixturesDir + "/default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499/chopper/0.log",
			}

			mockTailer1 := &mockTailer{}
			mockTailer2 := &mockTailer{}

			tracker := NewPodTracker(looper, disco, NewMockTailerFunc(&mockTailer{}), &mockFilter{}) // We don't use the func in this test
			tracker.LogTails = map[string]LogTailer{"file1": mockTailer1, "file2": mockTailer2}

			tracker.FlushOffsets()

			So(mockTailer1.FlushOffsetsWasCalled, ShouldBeTrue)
			So(mockTailer2.FlushOffsetsWasCalled, ShouldBeTrue)
		})
	})
}
