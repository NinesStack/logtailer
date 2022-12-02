package main

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

const fixturesDir = "fixtures"

func Test_NewDirListDiscoverer(t *testing.T) {
	Convey("NewDirListDiscoverer() properly configures a discoverer", t, func() {
		disco := NewDirListDiscoverer(fixturesDir, "dev")

		So(disco.Dir, ShouldEqual, fixturesDir)
	})
}

func Test_Discover(t *testing.T) {
	Convey("Discover()", t, func() {
		disco := NewDirListDiscoverer(fixturesDir, "dev")

		Convey("finds all the pods", func() {
			capture := LogCapture(func() {
				discovered, err := disco.Discover()
				So(err, ShouldBeNil)
				So(len(discovered), ShouldEqual, 6)
			})

			So(capture, ShouldNotContainSubstring, "Error")
		})

		Convey("fills out the details properly for each", func() {
			capture := LogCapture(func() {
				discovered, err := disco.Discover()
				So(err, ShouldBeNil)
				So(discovered[1].Name, ShouldEqual, "default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499")
				So(discovered[1].Namespace, ShouldEqual, "default")
				So(discovered[1].ServiceName, ShouldEqual, "chopper")
				So(discovered[1].Environment, ShouldEqual, "dev")

				So(discovered[2].Name, ShouldEqual, "default_pipeline-comparator-749f97cb4b-w8w4r_e5f10cd8-fb8a-4ade-b402-9b33f34f017f")
				So(discovered[2].Namespace, ShouldEqual, "default")
				So(discovered[2].ServiceName, ShouldEqual, "pipeline-comparator")
				So(discovered[2].Environment, ShouldEqual, "dev")
		})

			So(capture, ShouldNotContainSubstring, "Error")
		})

		Convey("errors when it can't open the dir", func() {
			disco := NewDirListDiscoverer("path-does-not-exist", "dev")
			discovered, err := disco.Discover()

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "discovery failed")
			So(discovered, ShouldBeNil)
		})
	})
}

func Test_LogFiles(t *testing.T) {
	Convey("LogFiles()", t, func() {
		disco := NewDirListDiscoverer(fixturesDir, "dev")

		Convey("finds all the files", func() {
			discovered, err := disco.LogFiles("default_pipeline-comparator-749f97cb4b-w8w4r_e5f10cd8-fb8a-4ade-b402-9b33f34f017f")

			So(err, ShouldBeNil)
			So(len(discovered), ShouldEqual, 3)
		})

		Convey("errors when the pod doesn't exist", func() {
			discovered, err := disco.LogFiles("doesn't exist")

			So(err, ShouldNotBeNil)
			So(discovered, ShouldBeNil)
		})
	})
}

func Test_namesFor(t *testing.T) {
	Convey("namesFor()", t, func() {
		disco := NewDirListDiscoverer(fixturesDir, "dev")

		Convey("handles names with dashes in them", func() {
			ns, serviceName, err := disco.namesFor("default_pipeline-comparator-749f97cb4b-w8w4r_e5f10cd8-fb8a-4ade-b402-9b33f34f017f")

			So(err, ShouldBeNil)
			So(ns, ShouldEqual, "default")
			So(serviceName, ShouldEqual, "pipeline-comparator")
		})

		Convey("handles names without dashes in them", func() {
			ns, serviceName, err := disco.namesFor("default_chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499")

			So(err, ShouldBeNil)
			So(ns, ShouldEqual, "default")
			So(serviceName, ShouldEqual, "chopper")
		})

		Convey("errors when it gets some garbage with too few fields", func() {
			ns, serviceName, err := disco.namesFor("default_chopper-f5b66c6bf-cgslk_")

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "failed to parse")
			So(ns, ShouldEqual, "")
			So(serviceName, ShouldEqual, "")
		})

		Convey("errors when it gets some malformed garbage", func() {
			ns, serviceName, err := disco.namesFor("default-chopper-f5b66c6bf-cgslk_9df92617-0407-470e-8182-a506aa7e0499")

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "failed to parse")
			So(ns, ShouldEqual, "")
			So(serviceName, ShouldEqual, "")
		})
	})
}
