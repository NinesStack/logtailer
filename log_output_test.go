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
