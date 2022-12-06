package reporter

// This originates from logproxy: https://github.com/Shimmur/logproxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	director "github.com/relistan/go-director"
	log "github.com/sirupsen/logrus"
)

// A LimitExeceededReporter track the current number of requests we have
// refused to proxy and reports them to New Relic on a 1 minute basis as
// an Insights event.
type LimitExceededReporter struct {
	client    *http.Client
	BaseURL   string
	InsertKey string
	AccountID string

	rateLimitedCount uint64
	ReportLooper     director.Looper
	hostname         string
}

// NewLimitExceededReporter returns a properly configured reporter
func NewLimitExceededReporter(url, insertKey, accountID string) *LimitExceededReporter {
	client := cleanhttp.DefaultClient()

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatal("Unable to determine hostname! Can't continue")
	}

	return &LimitExceededReporter{
		client:       client,
		BaseURL:      url,
		InsertKey:    insertKey,
		AccountID:    accountID,
		ReportLooper: director.NewTimedLooper(director.FOREVER, 1*time.Minute, make(chan error)),
		hostname:     hostname,
	}
}

// Incr atomically increments the current count
func (r *LimitExceededReporter) Incr() {
	atomic.AddUint64(&r.rateLimitedCount, 1)
}

// Run starts up a background goroutine that reports to New Relic on a 1 minute
// basis
func (r *LimitExceededReporter) Run() {
	log.Infof("Starting up New Relic reporter for account '%s'", r.AccountID)

	url := fmt.Sprintf("%s/%s/events", r.BaseURL, r.AccountID)

	go r.ReportLooper.Loop(func() error {
		// Get the current count, subtract it from the total using
		// atomic operations. This makes sure we don't lose any increments.
		count := atomic.LoadUint64(&r.rateLimitedCount)
		atomic.AddUint64(&r.rateLimitedCount, 0-count)

		if count > 0 {
			err := r.sendEvent(url, count)
			// We _don't_ want to exit on error
			if err != nil {
				log.Errorf("Error reporting to New Relic: %s", err)
			}
		}

		return nil
	})
}

// sentEvent serializes JSON and sends it to New Relic Insights
func (r *LimitExceededReporter) sendEvent(url string, count uint64) error {
	data, err := json.Marshal(struct {
		Time          string
		Hostname      string
		ExceededCount uint64
		EventType     string `json:"eventType"`
	}{
		Time:          time.Now().UTC().Format(time.RFC3339),
		Hostname:      r.hostname,
		ExceededCount: count,
		EventType:     "LogProxyRateLimitExceeded",
	})
	if err != nil {
		return fmt.Errorf("Unable to encode JSON event: %s", err)
	}

	buf := bytes.NewBuffer(data)
	req, err := http.NewRequest("POST", url, buf)
	if err != nil {
		return fmt.Errorf("Unable to create http request: %s", err)
	}
	req.Header.Add("X-Insert-Key", r.InsertKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("Failed making HTTP request to New Relic: %s", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("Bad response from New Relic: %s", string(body))
	}

	return nil
}
