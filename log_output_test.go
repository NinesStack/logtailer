package main

import (
	"errors"
	"net"
	"testing"
	"time"
	"encoding/json"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_UDPSyslogger(t *testing.T) {
	theJson := struct {
		Environment string    `json:"Environment"`
		Level       string    `json:"Level"`
		Payload     string    `json:"Payload"`
		ServiceName string    `json:"ServiceName"`
		Timestamp   time.Time `json:"Timestamp"`
	}{}

	Convey("UDPSyslogger()", t, func() {
		Convey("works end-to-end", func() {
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "bocaccio",
				"Environment": "medieval",
			}, "127.0.0.1:9714")

			logLine := "2022-12-06T12:20:28.418060579Z stdout F this is a test log line 💵 with UTF-8"

			go func() {
				logger.Log(logLine)
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
