package reporter

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/jarcoal/httpmock"
	director "github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewLimitExceededReporter(t *testing.T) {
	Convey("NewLimitExceededReporter() returns a properly configured struct", t, func() {
		url := "http://example.com"
		key := "mykey"
		account := "myaccount"
		reporter := NewLimitExceededReporter(url, key, account)

		So(reporter.BaseURL, ShouldEqual, url)
		So(reporter.InsertKey, ShouldEqual, key)
		So(reporter.AccountID, ShouldEqual, account)
		So(reporter.ReportLooper, ShouldNotBeNil)
		So(len(reporter.hostname), ShouldBeGreaterThan, 0)
		So(reporter.client, ShouldNotBeNil)
	})
}

func Test_Incr(t *testing.T) {
	Convey("Incr() increments the fail count", t, func() {
		url := "http://example.com"
		key := "mykey"
		account := "myaccount"
		reporter := NewLimitExceededReporter(url, key, account)

		reporter.Incr()
		reporter.Incr()

		So(reporter.rateLimitedCount, ShouldEqual, 2)
	})
}

func Test_Run(t *testing.T) {
	Convey("Run()", t, func() {
		Reset(func() {
			httpmock.DeactivateAndReset()
			log.SetOutput(ioutil.Discard)
		})

		capture := &bytes.Buffer{}
		log.SetOutput(capture)
		log.SetLevel(log.DebugLevel)

		url := "http://example.com"
		key := "mykey"
		account := "myaccount"
		reporter := NewLimitExceededReporter(url, key, account)
		httpmock.ActivateNonDefault(reporter.client)

		reporter.Incr()
		reporter.Incr()

		reporter.ReportLooper = director.NewFreeLooper(1, make(chan error))

		fullURL := url + "/" + account + "/events"

		hasHeader := false

		httpmock.RegisterResponder("POST", fullURL, func(req *http.Request) (*http.Response, error) {
			if req.Header["X-Insert-Key"][0] == key {
				hasHeader = true
			}
			return httpmock.NewStringResponse(200, `OK`), nil
		})

		Convey("Resets the counter", func() {
			So(reporter.rateLimitedCount, ShouldEqual, 2)
			reporter.Run()

			err := reporter.ReportLooper.Wait()
			So(err, ShouldBeNil)
			So(reporter.rateLimitedCount, ShouldEqual, 0)
		})

		Convey("Sends the event", func() {
			reporter.Run()
			err := reporter.ReportLooper.Wait()
			So(err, ShouldBeNil)

			httpmock.GetTotalCallCount()

			info := httpmock.GetCallCountInfo()
			So(info["POST "+fullURL], ShouldEqual, 1)
			So(hasHeader, ShouldBeTrue)
		})

		Convey("Doesn't send an event if the count is 0", func() {
			Reset(func() {
				// Don't interfere with the other tests
				reporter.Incr()
				reporter.Incr()
			})
			reporter.rateLimitedCount = 0
			reporter.Run()
			err := reporter.ReportLooper.Wait()
			So(err, ShouldBeNil)

			httpmock.GetTotalCallCount()

			info := httpmock.GetCallCountInfo()
			So(info["POST "+fullURL], ShouldEqual, 0)
		})

		Convey("Handles errors when New Relic is broken", func() {
			httpmock.RegisterResponder("POST", fullURL, func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(503, `Uh-oh`), nil
			})

			reporter.Run()
			err := reporter.ReportLooper.Wait()
			So(err, ShouldBeNil)

			So(capture.String(), ShouldContainSubstring, "Uh-oh")

			httpmock.GetTotalCallCount()

			info := httpmock.GetCallCountInfo()
			So(info["POST "+fullURL], ShouldEqual, 1)
		})
	})
}
