package main

import (
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Shimmur/logtailer/reporter"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_extractLogLevel(t *testing.T) {
	Convey("extractLogLevel()", t, func() {
		Convey("extracts info level from logfmt", func() {
			line := `time="2025-11-14T09:02:08Z" level=info msg="Started Worker" Namespace=workflow-automation`
			level, found := extractLogLevel(line)
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "info")
		})

		Convey("extracts error level from logfmt", func() {
			line := `time="2025-11-14T09:02:08Z" level=error msg="Something failed"`
			level, found := extractLogLevel(line)
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "error")
		})

		Convey("extracts warning level from logfmt", func() {
			line := `time="2025-11-14T09:36:09Z" level=warning msg="harvest failure" cmd=metric_data`
			level, found := extractLogLevel(line)
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "warning")
		})

		Convey("returns false when no level found", func() {
			line := `This is just a plain text log with no level`
			_, found := extractLogLevel(line)
			So(found, ShouldBeFalse)
		})
	})
}

func Test_UDPSyslogger(t *testing.T) {
	theJson := struct {
		Environment string    `json:"Environment"`
		Level       string    `json:"Level"`
		Payload     string    `json:"Payload"`
		ServiceName string    `json:"ServiceName"`
		Timestamp   time.Time `json:"Timestamp"`
		Container   string    `json:"Container"`
	}{}

	Convey("UDPSyslogger()", t, func() {
		Convey("works end-to-end", func() {
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "bocaccio",
				"Environment": "medieval",
			}, "127.0.0.1:9714")

			logLine := "2022-12-06T12:20:28.418060579Z stdout F this is a test log line 💵 with UTF-8"

			go func() {
				logger.Log(&LogLine{Text: logLine, Container: "beowulf"})
			}()

			received, err := ListenUDP("127.0.0.1:9714")
			So(err, ShouldBeNil)
			So(received, ShouldNotBeEmpty)

			err = json.Unmarshal(received, &theJson)
			So(err, ShouldBeNil)

			So(theJson.Environment, ShouldEqual, "medieval")
			So(theJson.ServiceName, ShouldEqual, "bocaccio")
			So(theJson.Payload, ShouldEqual, logLine[40:len(logLine)])
			So(theJson.Timestamp, ShouldNotBeEmpty)
			So(theJson.Container, ShouldEqual, "beowulf")
		})

		Convey("correctly parses level from structured logs on stderr", func() {
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "service",
				"Environment": "prod",
			}, "127.0.0.1:9715")

			// Info level log on stderr - should be logged as Info, not Error
			infoLog := `2025-11-14T09:02:08.322480471Z stderr F time="2025-11-14T09:02:08Z" level=info msg="Started Worker" Namespace=default`

			go func() {
				logger.Log(&LogLine{Text: infoLog, Container: "worker"})
			}()

			received, err := ListenUDP("127.0.0.1:9715")
			So(err, ShouldBeNil)
			So(received, ShouldNotBeEmpty)

			err = json.Unmarshal(received, &theJson)
			So(err, ShouldBeNil)

			// Should be logged as "info", NOT "error" despite being on stderr
			So(theJson.Level, ShouldEqual, "info")
		})

		Convey("correctly parses warning level from structured logs", func() {
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "service",
				"Environment": "prod",
			}, "127.0.0.1:9716")

			// Warning level log on stderr
			warnLog := `2025-11-14T09:36:09.227628554Z stderr F time="2025-11-14T09:36:09Z" level=warning msg="harvest failure" cmd=metric_data component=newrelic`

			go func() {
				logger.Log(&LogLine{Text: warnLog, Container: "worker"})
			}()

			received, err := ListenUDP("127.0.0.1:9716")
			So(err, ShouldBeNil)
			So(received, ShouldNotBeEmpty)

			err = json.Unmarshal(received, &theJson)
			So(err, ShouldBeNil)

			So(theJson.Level, ShouldEqual, "warning")
		})

		Convey("correctly handles error level logs", func() {
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "service",
				"Environment": "prod",
			}, "127.0.0.1:9717")

			// Error level log
			errorLog := `2025-11-14T09:02:08.322480471Z stderr F time="2025-11-14T09:02:08Z" level=error msg="Connection failed" error="timeout"`

			go func() {
				logger.Log(&LogLine{Text: errorLog, Container: "worker"})
			}()

			received, err := ListenUDP("127.0.0.1:9717")
			So(err, ShouldBeNil)
			So(received, ShouldNotBeEmpty)

			err = json.Unmarshal(received, &theJson)
			So(err, ShouldBeNil)

			// Should be logged as "error"
			So(theJson.Level, ShouldEqual, "error")
		})

		Convey("correctly handles quoted error level logs", func() {
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "service",
				"Environment": "prod",
			}, "127.0.0.1:9717")

			// Error level log
			errorLog := `2025-11-14T09:02:08.322480471Z stderr F time="2025-11-14T09:02:08Z" level="error" msg="Connection failed" error="timeout"`

			go func() {
				logger.Log(&LogLine{Text: errorLog, Container: "worker"})
			}()

			received, err := ListenUDP("127.0.0.1:9717")
			So(err, ShouldBeNil)
			So(received, ShouldNotBeEmpty)

			err = json.Unmarshal(received, &theJson)
			So(err, ShouldBeNil)

			// Should be logged as "error"
			So(theJson.Level, ShouldEqual, "error")
		})

		Convey("correctly handles unknown level logs as info", func() {
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "service",
				"Environment": "prod",
			}, "127.0.0.1:9717")

			// Error level log
			errorLog := `2025-11-14T09:02:08.322480471Z stderr F time="2025-11-14T09:02:08Z" level=unknown msg="Connection failed" error="timeout"`

			go func() {
				logger.Log(&LogLine{Text: errorLog, Container: "worker"})
			}()

			received, err := ListenUDP("127.0.0.1:9717")
			So(err, ShouldBeNil)
			So(received, ShouldNotBeEmpty)

			err = json.Unmarshal(received, &theJson)
			So(err, ShouldBeNil)

			// Should be logged as "error"
			So(theJson.Level, ShouldEqual, "info")
		})
	})
}

func Test_RateLimitingLogger(t *testing.T) {
	Convey("RateLimitingLogger", t, func() {
		rptr := reporter.NewLimitExceededReporter("", "", "")
		mockUpstream := &mockLogOutput{}
		logger := NewRateLimitingLogger(
			rptr, 1, 1*time.Millisecond, "ServiceName", mockUpstream,
		)

		Convey("can detect when logging has gone too far", func() {
			logger.Log(&LogLine{Text: "a line"})
			So(mockUpstream.WasCalled, ShouldBeTrue)
			mockUpstream.WasCalled = false

			logger.Log(&LogLine{Text: "a line 2"})
			So(mockUpstream.WasCalled, ShouldBeFalse)

			logger.Log(&LogLine{Text: "a line 3"})
			So(mockUpstream.WasCalled, ShouldBeFalse)
			So(mockUpstream.LastLogged, ShouldResemble, &LogLine{Text: "a line"})
		})
	})
}

func ListenUDP(address string) ([]byte, error) {
	pc, err := net.ListenPacket("udp", address)
	if err != nil {
		return nil, err
	}
	defer pc.Close()

	buf := make([]byte, 1024)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		return nil, err
	}

	if n < 1 {
		return nil, errors.New("received nothing")
	}

	return buf[:n], nil
}
