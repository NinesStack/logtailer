package cache

import (
	"io"
	"os"
	"testing"

	"github.com/nxadm/tail"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_EndToEnd(t *testing.T) {
	Convey("Testing end to end", t, func() {
		logFileName := "some/path/some-logfile-name"

		cacheFile, err := os.CreateTemp("", "seekInfoCache*")
		So(err, ShouldBeNil)

		Convey("Cache can write and reload from disk", func() {
			sought := &tail.SeekInfo{Offset: 10, Whence: io.SeekStart}
			sought2 := &tail.SeekInfo{Offset: 12, Whence: io.SeekStart}

			origCache := NewCache(5, cacheFile.Name())
			origCache.Add(logFileName, sought)
			origCache.Add(logFileName, sought2)

			err = origCache.Persist()
			So(err, ShouldBeNil)

			newCache := NewCache(5, cacheFile.Name())
			err = newCache.Load()
			So(err, ShouldBeNil)

			So(newCache.Get(logFileName), ShouldResemble, origCache.Get(logFileName))
		})

		Convey("Keys that are added are returned", func() {
			cache := NewCache(5, cacheFile.Name())
			sought := &tail.SeekInfo{Offset: 10, Whence: io.SeekStart}

			cache.Add("a filename", sought)

			So(cache.Get("a filename"), ShouldEqual, sought)
		})

		Convey("Keys that are deleted are not returned", func() {
			cache := NewCache(5, cacheFile.Name())
			sought := &tail.SeekInfo{Offset: 10, Whence: io.SeekStart}

			cache.Add("a filename", sought)
			cache.Del("a filename")

			So(cache.Get("a filename"), ShouldBeNil)
		})
	})
}
