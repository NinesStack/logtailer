package main

import (
	"io/ioutil"
	"strings"

	"github.com/Nitro/sidecar-executor/loghooks"
	log "github.com/sirupsen/logrus"
)

type LogOutput interface {
	Log(line string)
}

type UDPSyslogger struct {
	syslogger *log.Entry
}

func NewUDPSyslogger(labels map[string]string, address string) *UDPSyslogger {
	syslogger := log.New()

	// We relay UDP syslog because we don't plan to ship it off the box and
	// because it's simplest since there is no backpressure issue to deal with.
	hook, err := loghooks.NewUDPHook(address)
	if err != nil {
		log.Fatalf("Error adding hook: %s", err)
	}

	syslogger.Hooks.Add(hook)
	syslogger.SetFormatter(&log.JSONFormatter{
		FieldMap: log.FieldMap{
			log.FieldKeyTime:  "Timestamp",
			log.FieldKeyLevel: "Level",
			log.FieldKeyMsg:   "Payload",
			log.FieldKeyFunc:  "Func",
		},
	})
	syslogger.SetOutput(ioutil.Discard)

	// Add one to the labels length to account for hostname
	fields := make(log.Fields, len(labels)+1)

	// Loop through the fields we're supposed to pass, and add them
	for field, val := range labels {
		fields[field] = val
	}

	return &UDPSyslogger{
		syslogger: syslogger.WithFields(fields),
	}
}

// relayLogs will watch a container and send the logs to Syslog
func (sysl *UDPSyslogger) Log(line string) {
	// 2022-12-03T16:09:51.741778906Z stdout F

	k8sFields := strings.Split(line[0:40], " ")
	descriptor := k8sFields[1]

	// Attempt to detect errors to log (a la sidecar-executor)
	if descriptor == "stderr" || strings.Contains(strings.ToLower(line), "error") {
		sysl.syslogger.Error(line[40 : len(line)-1])
		return
	}

	sysl.syslogger.Info(line)
}
