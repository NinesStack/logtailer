package main

import (
	"io/ioutil"
	"os"
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
			err := tailer.TailLogs(logFiles)
			So(err, ShouldBeNil)

			// Make sure we are tracking more than one file
			So(len(tailer.LogTails), ShouldEqual, 3)
			So(tailer.LogTails[0].Filename, ShouldContainSubstring, "chopper/0.log")
			So(tailer.LogTails[1].Filename, ShouldContainSubstring, "logproxy/0.log")
			So(tailer.LogTails[2].Filename, ShouldContainSubstring, "vault-init/0.log")

			go tailer.Run()
			defer tailer.Stop()


			// Nothing should be cached yet
			So(len(tailer.localCache), ShouldEqual, 0)

			// Put something into the logfiles
			for _, tail := range tailer.LogTails {
				logF, err := os.OpenFile(tail.Filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				So(err, ShouldBeNil)
				logF.WriteString("this is a test message\n")
				logF.Close()
			}

			// Janky, but we have to wait for the files to flush to the tail
			time.Sleep(100 * time.Millisecond)

			// Now we should know about all of their offsets
			So(len(tailer.localCache), ShouldEqual, 3)

			So(logOutput.CallCount, ShouldEqual, 3)
		})
	})
}
