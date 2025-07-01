package main

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Shimmur/logtailer/cache"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_NewTailer(t *testing.T) {
	Convey("NewTailer()", t, func() {
		pod := &Pod{Name: "venerable bede"}
		cache := cache.NewCache(5, "/tmp/testcache")
		logOutput := &mockLogOutput{}

		tailer := NewTailer(pod, cache, logOutput)

		So(tailer.Pod, ShouldEqual, pod)
		So(tailer.LogChan, ShouldNotBeNil)
		So(tailer.logger, ShouldEqual, logOutput)
		So(tailer.localCache, ShouldNotBeNil)
		So(tailer.looper, ShouldNotBeNil)
	})
}

func Test_TailLogs(t *testing.T) {
	Convey("TailLogs()", t, func() {
		disco := NewDirListDiscoverer(fixturesDir, "dev")
		pod := &Pod{Name: "venerable bede"}
		cache := cache.NewCache(5, "/tmp/testcache")
		logOutput := &mockLogOutput{}

		tailer := NewTailer(pod, cache, logOutput)

		pods, err := disco.Discover()
		So(err, ShouldBeNil)
		So(pods, ShouldNotBeEmpty)

		// Make sure we have the chopper fixture, which has more than one logfile
		podName := pods[1].Name
		So(podName, ShouldEqual, "default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499")

		// Get chopper's logs
		logFiles, err := disco.LogFiles(podName)
		So(err, ShouldBeNil)
		So(logFiles, ShouldNotBeEmpty)

		Reset(func() {
			// Empty the fixture files
			for _, tail := range tailer.LogTails {
				_ = ioutil.WriteFile(tail.Filename, []byte{}, 0644)
			}
		})

		Convey("opens tails on all the logs and caches them", func() {
			_ = LogCapture(func() {
				err := tailer.TailLogs(logFiles)
				So(err, ShouldBeNil)
			})

			// Make sure we are tracking more than one file
			So(len(tailer.LogTails), ShouldEqual, 4)
			So(
				tailer.LogTails["fixtures/pods/default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499/chopper/0.log"],
				ShouldNotBeNil,
			)
			So(
				tailer.LogTails["fixtures/pods/default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499/chopper/1.log"],
				ShouldNotBeNil,
			)
			So(
				tailer.LogTails["fixtures/pods/default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499/logproxy/0.log"],
				ShouldNotBeNil,
			)
			So(
				tailer.LogTails["fixtures/pods/default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499/vault-init/0.log"],
				ShouldNotBeNil,
			)

			tailer.Run()
			Reset(tailer.Stop)

			// Nothing should be cached yet
			So(len(tailer.localCache), ShouldEqual, 0)

			// Put something into the logfiles
			for _, tail := range tailer.LogTails {
				logF, err := os.OpenFile(tail.Filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				So(err, ShouldBeNil)
				logF.WriteString("this is a test message\n")
				logF.Close()
			}

			timeout := time.After(300 * time.Millisecond)
			// We have to wait for the files to flush to the tail
			for {
				select {
				case <-timeout:
					So("we should have received something", ShouldNotBeEmpty)
				default: // keep going
				}
				time.Sleep(1 * time.Millisecond)
				if logOutput.LastLogged != nil {
					break
				}
			}

			// Now we should know about all of their offsets
			tailer.lock.RLock()
			So(len(tailer.localCache), ShouldEqual, 4)
			tailer.lock.RUnlock()

			logOutput.Lock()
			defer logOutput.Unlock()
			So(logOutput.CallCount, ShouldEqual, 4)
		})

		Convey("passes on shutdown message to the log output", func() {
			tailer.Run()
			tailer.Stop()

			So(logOutput.StopWasCalled, ShouldBeTrue)
		})

		Convey("removes logs we're no longer seeing", func() {
			_ = LogCapture(func() {
				err := tailer.TailLogs(logFiles)
				So(err, ShouldBeNil)
			})

			So(len(tailer.LogTails), ShouldEqual, 4)

			logFiles = logFiles[1:3]
			capture := LogCapture(func() {
				err := tailer.TailLogs(logFiles)
				So(err, ShouldBeNil)
			})

			numberOfDroppedLogs := strings.Count(capture, "Dropping tail")
			So(numberOfDroppedLogs, ShouldEqual, 2)
			So(len(tailer.LogTails), ShouldEqual, 2)
		})

		Convey("extracts and logs the container name", func() {
			_ = LogCapture(func() {
				err := tailer.TailLogs(logFiles)
				So(err, ShouldBeNil)

				tailer.Run()
			})
			Reset(tailer.Stop)

			// Only send on one of the logs, so we can check the resulting container name
			for _, tail := range tailer.LogTails {
				if strings.Contains(tail.Filename, "vault-init") {
					logF, err := os.OpenFile(tail.Filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					So(err, ShouldBeNil)
					logF.WriteString("this is a test message\n")
					logF.Close()
				}
			}

			// We have to wait for the files to flush to the tail
			timeout := time.After(300 * time.Millisecond)
			for {
				select {
				case <-timeout:
					So("we should have received something", ShouldNotBeEmpty)
				default: // keep going
				}
				time.Sleep(1 * time.Millisecond)
				if logOutput.LastLogged != nil {
					break
				}
			}

			So(logOutput.LastLogged, ShouldNotBeNil)
			So(logOutput.LastLogged.Text, ShouldEqual, "this is a test message")
			So(logOutput.LastLogged.Container, ShouldEqual, "vault-init")
		})
	})
}
