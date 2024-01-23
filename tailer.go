package main

import (
	"fmt"
	"sync"

	"github.com/Shimmur/logtailer/cache"
	"github.com/nxadm/tail"
	director "github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
)

// LogTailer defines the interface expected by a PodTracker
type LogTailer interface {
	TailLogs(logFiles []string) error
	Run()
	FlushOffsets()
	Stop()
}

// A Tailer watches all the logs for a Pod
type Tailer struct {
	LogTails  []*tail.Tail
	Filenames []string
	Pod       *Pod
	LogChan   chan *tail.Line

	logger LogOutput

	looper     director.Looper
	cache      *cache.Cache
	localCache map[string]*tail.SeekInfo
	lock       sync.RWMutex
}

// NewTailer returns a properly configured Tailer for a Pod
func NewTailer(pod *Pod, cache *cache.Cache, logger LogOutput) *Tailer {
	return &Tailer{
		Pod:        pod,
		LogChan:    make(chan *tail.Line),
		looper:     director.NewFreeLooper(director.FOREVER, make(chan error)),
		cache:      cache,
		localCache: make(map[string]*tail.SeekInfo, 5),
		logger:     logger,
	}
}

// TailLogs takes a list of filenames and opens a tail on them. The logs from
// the tail are copied into the main LogChan. This is then processed when Run()
// is invoked. The channels are all unbuffered.
func (t *Tailer) TailLogs(logFiles []string) error {
	for _, filename := range logFiles {
		tailed, err := t.tailOneLog(filename)

		if err != nil {
			// We have to clean up all the tails that started already
			for _, tailed := range t.LogTails {
				_ = tailed.Stop() // Ignore any errors
			}
			close(t.LogChan)
			return fmt.Errorf("failed to tail log for %s: %w", t.Pod.Name, err)
		}

		// Copy into the main channel. These will exit when the tail is
		// stopped.
		go t.logPump(filename, tailed)
	}

	return nil
}

// tailOneLog will setup a tailer for a logfile and fire off a background log
// pump, to push logs into the main channel.
func (t *Tailer) tailOneLog(filename string) (*tail.Tail, error) {
	tailConfig := tail.Config{
		ReOpen: true, Follow: true, Logger: log.StandardLogger(), Location: nil,
		MustExist: true, Poll: true,
	}

	// Try to get an existing offset from the main cache
	if sought := t.cache.Get(filename); sought != nil {
		log.Infof("  Found existing offset for %s, skipping to position", filename)
		tailConfig.Location = sought
	}

	tailed, err := tail.TailFile(filename, tailConfig)
	if err != nil {
		log.Warnf("Error tailing %s for pod %s: %s", filename, t.Pod.Name, err)
		return nil, err
	}

	log.Infof("  Adding tail on %s for pod %s", filename, t.Pod.Name)
	t.LogTails = append(t.LogTails, tailed)

	return tailed, nil
}

// logPump runs in a goroutine for each log file, copying logs into the main
// channel.
func (t *Tailer) logPump(filename string, tailed *tail.Tail) {
	for l := range tailed.Lines {
		t.LogChan <- l
		t.localCacheAdd(filename, &(l.SeekInfo))
	}
	log.Infof("  Closing tail on %s for pod %s", filename, t.Pod.Name)
}

// Run processes all the logs currently pending, and then writes the current
// seek info for each log to the main cache for persistence.
func (t *Tailer) Run() {
	go t.looper.Loop(func() error {
		for line := range t.LogChan {
			t.logger.Log(line.Text)
		}
		return nil
	})
}

// FlushOffests writes all the offsets from the localCache into the main cache.
// This is triggered from the PodTracker.
func (t *Tailer) FlushOffsets() {
	t.lock.RLock()
	defer t.lock.RUnlock()
	// Write our local cache to the main cache, from which it will be persisted.
	// Prevents the lock on the main cache from bottlenecking all log flushes.
	for filename, seekInfo := range t.localCache {
		log.Infof("Flushing offsets for %s", filename)
		t.cache.Add(filename, seekInfo)
	}
}

func (t *Tailer) Stop() {
	for _, entry := range t.LogTails {
		err := entry.Stop()
		if err != nil {
			log.Errorf("Failed to stop tail for pod %s: %s", t.Pod.Name, err)
		}

		// Remove any inotify watches
		entry.Cleanup()
	}

	// Remove our offsets from the persisted cache
	for filename, _ := range t.localCache {
		t.cache.Del(filename)
	}

	t.logger.Stop()

	t.looper.Quit()
}

func (t *Tailer) localCacheAdd(filename string, seekInfo *tail.SeekInfo) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.localCache[filename] = seekInfo // Cache locally
}
