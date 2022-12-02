package main

import (
	"fmt"

	"github.com/nxadm/tail"
	director "github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
)

type Tailer struct {
	LogTails []*tail.Tail
	Pod      *Pod
	LogChan  chan *tail.Line

	looper director.Looper
}

// NewTailer returns a properly configured Tailer for a Pod
func NewTailer(pod *Pod) *Tailer {
	return &Tailer{
		Pod:     pod,
		LogChan: make(chan *tail.Line),
		looper:  director.NewFreeLooper(director.FOREVER, make(chan error)),
	}
}

// TailLogs takes a list of filenames and opens a tail on them. The logs from
// the tail are copied into the main LogChan. This is then processed when Run()
// is invoked. The channels are all unbuffered.
func (t *Tailer) TailLogs(logFiles []string) error {
	var (
		failed bool
		err    error
		tailed *tail.Tail
	)

	for _, filename := range logFiles {
		tailed, err = tail.TailFile(filename, tail.Config{ReOpen: true, Follow: true, Logger: log.StandardLogger()})
		if err != nil {
			failed = true
		}

		log.Infof("  Adding tail on %s for pod %s", filename, t.Pod.Name)
		t.LogTails = append(t.LogTails, tailed)

		// Copy into the main channel. These till exit when the tail is stopped.
		go func() {
			for l := range tailed.Lines {
				t.LogChan <- l
			}
		}()
	}

	if failed {
		// We have to clean up all the tails that started already
		for _, tailed := range t.LogTails {
			_ = tailed.Stop() // Ignore any errors
		}
		close(t.LogChan)
		return fmt.Errorf("failed to tail log for %s: %w", t.Pod.Name, err)
	}

	return nil
}

func (t *Tailer) Run() {
	t.looper.Loop(func() error {
		for line := range t.LogChan {
			// TODO rate limit and send UDP
			println(line.Text)
			// TODO on a timed basis we could track seek offset to a file so restarts
			// don't flush the whole log file
		}
		return nil
	})
}

func (t *Tailer) Stop() {
	for _, entry := range t.LogTails {
		err := entry.Stop()
		if err != nil {
			log.Errorf("Failed to stop tail for pod %s: %s", t.Pod.Name, err)
		}
	}
	t.looper.Quit()
}
