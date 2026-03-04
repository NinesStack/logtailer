package main

// The bulk of this comes from https://github.com/NinesStack/sidecar

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	log "github.com/sirupsen/logrus"
)

type K8sPodsMetadata struct {
	Items []struct {
		Metadata struct {
			Annotations struct {
				CommunityComTailLogs string `json:"community.com/TailLogs"` // Backwards compatibility
				LogtailerTailLogs    string `json:"logtailer/TailLogs"`
				LogtailerLogFormat   string `json:"logtailer/LogFormat"`
			} `json:"annotations"`
		} `json:"metadata"`
	} `json:"items"`
}

// A PodFilter calls out to the Kubernetes API and determines if annotations
// are present on a pod that would enable us to track logs for that pod.
type PodFilter struct {
	Timeout time.Duration

	KubeHost string
	KubePort int

	credsPath string
	token     string
	client    *http.Client
}

func NewPodFilter(kubeHost string, kubePort int, timeout time.Duration, credsPath string) *PodFilter {
	f := &PodFilter{
		Timeout:   timeout,
		KubeHost:  kubeHost,
		KubePort:  kubePort,
		credsPath: credsPath,
	}
	// Cache the secret from the file
	if err := f.refreshToken(); err != nil {
		log.Errorf("Failed to read serviceaccount token: %s", err)
		return nil
	}

	// Set up the timeout on a clean HTTP client
	f.client = cleanhttp.DefaultClient()
	f.client.Timeout = f.Timeout

	// Get the SystemCertPool — on error we have empty pool
	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	certs, err := ioutil.ReadFile(credsPath + "/ca.crt")
	if err != nil {
		log.Warnf("Failed to load CA cert file: %s", err)
	}

	if ok := rootCAs.AppendCertsFromPEM(certs); !ok {
		log.Warn("No certs appended! Using system certs only")
	}

	// Add the pool to the TLS config we'll use in the client.
	config := &tls.Config{
		RootCAs: rootCAs,
	}

	f.client.Transport = &http.Transport{TLSClientConfig: config}

	return f
}

// refreshToken re-reads the service account token from disk. The kubelet
// rotates this file before the token expires.
func (f *PodFilter) refreshToken() error {
	data, err := os.ReadFile(f.credsPath + "/token")
	if err != nil {
		return fmt.Errorf("failed to read serviceaccount token: %w", err)
	}
	// Newline is illegal in tokens
	f.token = strings.Replace(string(data), "\n", "", -1)
	return nil
}

func (f *PodFilter) newRequest(urlStr string) (*http.Request, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "logtailer/"+Version)
	req.Header.Set("Authorization", "Bearer "+f.token)
	return req, nil
}

func (f *PodFilter) makeRequest(path string) ([]byte, error) {
	var scheme = "http"
	if f.KubePort == 443 {
		scheme = "https"
	}

	// Start with the path, then add the host and scheme
	apiURL, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("unable to parse the path! %s: %w", path, err)
	}
	apiURL.Scheme = scheme
	apiURL.Host = fmt.Sprintf("%s:%d", f.KubeHost, f.KubePort)

	req, err := f.newRequest(apiURL.String())
	if err != nil {
		return []byte{}, err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to fetch from K8s API '%s': %w", path, err)
	}

	// On 401, refresh the token from disk and retry once
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()

		log.Warnf("Got 401 from K8s API for '%s', refreshing service account token", path)
		if err := f.refreshToken(); err != nil {
			return []byte{}, fmt.Errorf("failed to refresh token after 401: %w", err)
		}

		req, err = f.newRequest(apiURL.String())
		if err != nil {
			return []byte{}, err
		}

		resp, err = f.client.Do(req)
		if err != nil {
			return []byte{}, fmt.Errorf("failed to fetch from K8s API '%s' after token refresh: %w", path, err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 || resp.StatusCode < 200 {
		return []byte{}, fmt.Errorf("got unexpected response code from %s: %d", path, resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to read from K8s API '%s' response body: %w", path, err)
	}

	return body, nil
}

func (f *PodFilter) ShouldTailLogs(pod *Pod) (bool, error) {
	body, err := f.makeRequest(
		"/api/v1/namespaces/" + pod.Namespace + "/pods?limit=100000&labelSelector=ServiceName%3D" + pod.ServiceName,
	)
	if err != nil {
		return false, err
	}

	var pods K8sPodsMetadata
	err = json.Unmarshal(body, &pods)
	if err != nil {
		return false, fmt.Errorf("unable to decode response from K8s: %s", err)
	}

	// We don't somehow know about this pod (yet)
	if len(pods.Items) < 1 {
		return false, nil
	}

	// If *ANY* of the pods enables logs, we enable for all of them
	for _, podItem := range pods.Items {
		if podItem.Metadata.Annotations.CommunityComTailLogs == "true" || podItem.Metadata.Annotations.LogtailerTailLogs == "true" {
			// Capture the log format annotation if present
			if podItem.Metadata.Annotations.LogtailerLogFormat != "" {
				pod.LogFormat = podItem.Metadata.Annotations.LogtailerLogFormat
			}
			return true, nil
		}
	}

	return false, nil
}

// A StubFilter is used when we fail to talk to Kubernetes, e.g. when
// running locally.
type StubFilter struct{}

func (f *StubFilter) ShouldTailLogs(pod *Pod) (bool, error) { return true, nil }
