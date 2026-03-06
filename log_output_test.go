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

func Test_extractLogLevelWithRegex(t *testing.T) {
	Convey("extractLogLevelWithRegex()", t, func() {
		Convey("extracts info level from logfmt", func() {
			line := `time="2025-11-14T09:02:08Z" level=info msg="Started Worker" Namespace=workflow-automation`
			level, found := extractLogLevelWithRegex(line, selectLogRegex(LogFormatGo))
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "info")
		})

		Convey("extracts error level from logfmt", func() {
			line := `time="2025-11-14T09:02:08Z" level=error msg="Something failed"`
			level, found := extractLogLevelWithRegex(line, selectLogRegex(LogFormatGo))
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "error")
		})

		Convey("extracts warning level from logfmt", func() {
			line := `time="2025-11-14T09:36:09Z" level=warning msg="harvest failure" cmd=metric_data`
			level, found := extractLogLevelWithRegex(line, selectLogRegex(LogFormatGo))
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "warning")
		})

		Convey("returns false when no level found", func() {
			line := `This is just a plain text log with no level`
			_, found := extractLogLevelWithRegex(line, selectLogRegex(LogFormatGo))
			So(found, ShouldBeFalse)
		})

		Convey("extracts info level from JSON format", func() {
			line := `{"level":"info","ts":"2025-12-18T07:15:30Z","logger":"controllers.ingress","msg":"successfully deployed model","ingressGroup":"k8s-internal-dev"}`
			level, found := extractLogLevelWithRegex(line, selectLogRegex(LogFormatJSON))
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "info")
		})

		Convey("extracts error level from JSON format", func() {
			line := `{"level":"error","ts":"2025-12-18T07:15:30Z","logger":"controllers.ingress","msg":"deployment failed"}`
			level, found := extractLogLevelWithRegex(line, selectLogRegex(LogFormatJSON))
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "error")
		})

		Convey("extracts warn level from JSON format", func() {
			line := `{"level":"warn","ts":"2025-12-18T07:15:30Z","logger":"controllers.ingress","msg":"deprecated API used"}`
			level, found := extractLogLevelWithRegex(line, selectLogRegex(LogFormatJSON))
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "warn")
		})

		Convey("extracts info level from tab-separated format", func() {
			line := `2025-12-18T06:20:47.312Z    INFO    main    Bootstrap    memory-manager.http-client.log.path`
			level, found := extractLogLevelWithRegex(line, selectLogRegex(LogFormatTab))
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "info")
		})

		Convey("extracts error level from tab-separated format", func() {
			line := `2025-12-18T06:20:47.312Z    ERROR    main    Bootstrap    Failed to initialize`
			level, found := extractLogLevelWithRegex(line, selectLogRegex(LogFormatTab))
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "error")
		})

		Convey("extracts warning level from tab-separated format", func() {
			line := `2025-12-18T06:20:47.312Z    WARN    main    Bootstrap    Deprecated configuration`
			level, found := extractLogLevelWithRegex(line, selectLogRegex(LogFormatTab))
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "warn")
		})
	})
}

func Test_containsWord(t *testing.T) {
	Convey("containsWord()", t, func() {
		Convey("does not match --show-error flag", func() {
			So(containsWord("++ curl --show-error --fail", "error"), ShouldBeFalse)
		})

		Convey("does not match --warn-something flag", func() {
			So(containsWord("--warn-something", "warn"), ShouldBeFalse)
		})

		Convey("matches standalone error", func() {
			So(containsWord("an error occurred", "error"), ShouldBeTrue)
		})

		Convey("matches warn followed by colon", func() {
			So(containsWord("warn: something bad", "warn"), ShouldBeTrue)
		})

		Convey("matches error in brackets", func() {
			So(containsWord("[error] failed", "error"), ShouldBeTrue)
		})

		Convey("matches error as entire string", func() {
			So(containsWord("error", "error"), ShouldBeTrue)
		})

		Convey("does not match error inside a word", func() {
			So(containsWord("myerrorhandler", "error"), ShouldBeFalse)
		})
	})
}

func Test_selectLogRegex_ValidFormats(t *testing.T) {
	Convey("selectLogRegex() with valid formats", t, func() {
		Convey("LogFormatGo returns go-logfmt format", func() {
			regex := selectLogRegex(LogFormatGo)
			So(regex, ShouldEqual, logFormatRegexes[LogFormatGo])

			// Verify it actually works by testing with a logfmt line
			line := `level=info msg="test"`
			level, found := extractLogLevelWithRegex(line, regex)
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "info")
		})

		Convey("LogFormatJSON returns json format", func() {
			regex := selectLogRegex(LogFormatJSON)
			So(regex, ShouldEqual, logFormatRegexes[LogFormatJSON])

			// Verify it actually works
			line := `{"level":"warn","msg":"test"}`
			level, found := extractLogLevelWithRegex(line, regex)
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "warn")
		})

		Convey("LogFormatTab returns tab-separated format", func() {
			regex := selectLogRegex(LogFormatTab)
			So(regex, ShouldEqual, logFormatRegexes[LogFormatTab])

			// Verify it actually works
			line := `2025-12-18T06:20:47.312Z    ERROR    main    test`
			level, found := extractLogLevelWithRegex(line, regex)
			So(found, ShouldBeTrue)
			So(level, ShouldEqual, "error")
		})
	})
}

func Test_parseLogFormat(t *testing.T) {
	Convey("parseLogFormat()", t, func() {
		Convey("empty string returns LogFormatGo", func() {
			So(parseLogFormat(""), ShouldEqual, LogFormatGo)
		})

		Convey("go-logfmt returns LogFormatGo", func() {
			So(parseLogFormat("go-logfmt"), ShouldEqual, LogFormatGo)
		})

		Convey("json returns LogFormatJSON", func() {
			So(parseLogFormat("json"), ShouldEqual, LogFormatJSON)
		})

		Convey("tab returns LogFormatTab", func() {
			So(parseLogFormat("tab"), ShouldEqual, LogFormatTab)
		})

		Convey("ruby returns LogFormatRuby", func() {
			So(parseLogFormat("ruby"), ShouldEqual, LogFormatRuby)
		})

		Convey("invalid format logs warning and falls back to LogFormatGo", func() {
			output := LogCapture(func() {
				format := parseLogFormat("xml")
				So(format, ShouldEqual, LogFormatGo)
			})
			So(output, ShouldContainSubstring, "Unsupported log format 'xml'")
			So(output, ShouldContainSubstring, "falling back to 'go-logfmt'")
		})

		Convey("wrong case format logs warning and falls back", func() {
			output := LogCapture(func() {
				format := parseLogFormat("JSON")
				So(format, ShouldEqual, LogFormatGo)
			})
			So(output, ShouldContainSubstring, "Unsupported log format 'JSON'")
			So(output, ShouldContainSubstring, "falling back to 'go-logfmt'")
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
		LogFormat   string    `json:"LogFormat"`
	}{}

	Convey("UDPSyslogger()", t, func() {
		Convey("works end-to-end", func() {
			enableRegexLogLevelParsing := false
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "bocaccio",
				"Environment": "medieval",
			}, "127.0.0.1:9714", enableRegexLogLevelParsing, "") // enhanced regex parsing disabled to test og mode

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
			So(theJson.LogFormat, ShouldEqual, "") // Should be empty when regex parsing disabled
		})

		Convey("correctly parses level from structured logs on stderr", func() {
			enableRegexLogLevelParsing := true
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "service",
				"Environment": "prod",
			}, "127.0.0.1:9715", enableRegexLogLevelParsing, "")

			// Info level info on stderr - should be logged as Info, not Error
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
			So(theJson.LogFormat, ShouldEqual, "go-logfmt") // Should default to go-logfmt when empty
		})

		Convey("correctly parses warning level from structured logs", func() {
			enableRegexLogLevelParsing := true
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "service",
				"Environment": "prod",
			}, "127.0.0.1:9716", enableRegexLogLevelParsing, "")

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
			enableRegexLogLevelParsing := true
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "service",
				"Environment": "prod",
			}, "127.0.0.1:9717", enableRegexLogLevelParsing, "")

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
			enableRegexLogLevelParsing := true
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "service",
				"Environment": "prod",
			}, "127.0.0.1:9717", enableRegexLogLevelParsing, "")

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
			enableRegexLogLevelParsing := true
			logger := NewUDPSyslogger(map[string]string{
				"ServiceName": "service",
				"Environment": "prod",
			}, "127.0.0.1:9717", enableRegexLogLevelParsing, "")

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
