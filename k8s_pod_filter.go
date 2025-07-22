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
	"strings"
	"time"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	log "github.com/sirupsen/logrus"
)

type K8sPodsMetadata struct {
	Items []struct {
		Metadata struct {
			Annotations struct {
				CommunityComTailLogs string `json:"community.com/TailLogs"`
			} `json:"annotations"`
		} `json:"metadata"`
	} `json:"items"`
}

// An AnnotationPodFilter calls out to the Kubernetes API and determines if annotations
// are present on a pod that would enable us to track logs for that pod.
type AnnotationPodFilter struct {
	Timeout time.Duration

	KubeHost string
	KubePort int

	token  string
	client *http.Client
}

func NewAnnotationPodFilter(kubeHost string, kubePort int, timeout time.Duration, credsPath string) *AnnotationPodFilter {
	f := &AnnotationPodFilter{
		Timeout:  timeout,
		KubeHost: kubeHost,
		KubePort: kubePort,
	}
	// Cache the secret from the file
	data, err := ioutil.ReadFile(credsPath + "/token")
	if err != nil {
		log.Errorf("Failed to read serviceaccount token: %s", err)
		return nil
	}

	// New line is illegal in tokens
	f.token = strings.Replace(string(data), "\n", "", -1)

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

func (f *AnnotationPodFilter) makeRequest(path string) ([]byte, error) {
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

	req, err := http.NewRequest("GET", apiURL.String(), nil)
	if err != nil {
		return []byte{}, err
	}

	req.Header.Set("Authorization", "Bearer "+f.token)

	resp, err := f.client.Do(req)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to fetch from K8s API '%s': %w", path, err)
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

// extractPodName extracts the Kubernetes pod name from the directory name
// e.g., "default_zapier-integration-c7fdf5784-rlx4x_f3a8f5d2-a2ed-48d3-9753-e661919d6f7b"
// -> "zapier-integration-c7fdf5784-rlx4x"
func extractPodName(dirName string) string {
	// Remove the namespace prefix
	if idx := strings.Index(dirName, "_"); idx != -1 {
		dirName = dirName[idx+1:]
	}

	// Remove the UUID suffix (everything after the last underscore)
	if idx := strings.LastIndex(dirName, "_"); idx != -1 {
		dirName = dirName[:idx]
	}

	return dirName
}

func (f *AnnotationPodFilter) ShouldTailLogs(pod *Pod) (bool, error) {
	// Try querying by ServiceName first (with underscore version)
	serviceNameLabel := strings.Replace(pod.ServiceName, "-", "_", -1)
	body, err := f.makeRequest(
		"/api/v1/namespaces/" + pod.Namespace + "/pods?limit=100000&labelSelector=ServiceName%3D" + serviceNameLabel,
	)
	if err != nil {
		return false, err
	}

	var pods K8sPodsMetadata
	err = json.Unmarshal(body, &pods)
	if err != nil {
		return false, fmt.Errorf("unable to decode response from K8s: %s", err)
	}

	// If no pods found by ServiceName, try querying the specific pod by name
	if len(pods.Items) < 1 {
		body, err = f.makeRequest(
			"/api/v1/namespaces/" + pod.Namespace + "/pods/" + extractPodName(pod.Name),
		)
		if err != nil {
			return false, err
		}

		var singlePod struct {
			Metadata struct {
				Annotations struct {
					CommunityComTailLogs string `json:"community.com/TailLogs"`
				} `json:"annotations"`
			} `json:"metadata"`
		}
		err = json.Unmarshal(body, &singlePod)
		if err != nil {
			return false, fmt.Errorf("unable to decode response from K8s: %s", err)
		}

		if singlePod.Metadata.Annotations.CommunityComTailLogs == "true" {
			return true, nil
		}
		return false, nil
	}

	// If *ANY* of the pods enables logs, we enable for all of them
	for _, podItem := range pods.Items {
		if podItem.Metadata.Annotations.CommunityComTailLogs == "true" {
			return true, nil
		}
	}

	return false, nil
}

// A TailAllFilter reuses AnnotationPodFilter but inverts the logic - tail all pods except those with TailLogs=false
type TailAllFilter struct {
	*AnnotationPodFilter
}

func NewTailAllFilter(kubeHost string, kubePort int, timeout time.Duration, credsPath string) *TailAllFilter {
	annotationFilter := NewAnnotationPodFilter(kubeHost, kubePort, timeout, credsPath)
	if annotationFilter == nil {
		return nil
	}
	return &TailAllFilter{AnnotationPodFilter: annotationFilter}
}

func (f *TailAllFilter) ShouldTailLogs(pod *Pod) (bool, error) {
	// Try querying by ServiceName first (with underscore version)
	serviceNameLabel := strings.Replace(pod.ServiceName, "-", "_", -1)
	body, err := f.makeRequest(
		"/api/v1/namespaces/" + pod.Namespace + "/pods?limit=100000&labelSelector=ServiceName%3D" + serviceNameLabel,
	)
	if err != nil {
		return false, err
	}

	var pods K8sPodsMetadata
	err = json.Unmarshal(body, &pods)
	if err != nil {
		return false, fmt.Errorf("unable to decode response from K8s: %s", err)
	}

	// If no pods found by ServiceName, try querying the specific pod by name
	if len(pods.Items) < 1 {
		body, err = f.makeRequest(
			"/api/v1/namespaces/" + pod.Namespace + "/pods/" + extractPodName(pod.Name),
		)
		if err != nil {
			return false, err
		}

		var singlePod struct {
			Metadata struct {
				Annotations struct {
					CommunityComTailLogs string `json:"community.com/TailLogs"`
				} `json:"annotations"`
			} `json:"metadata"`
		}
		err = json.Unmarshal(body, &singlePod)
		if err != nil {
			return false, fmt.Errorf("unable to decode response from K8s: %s", err)
		}

		// In TailAll mode: tail unless explicitly disabled
		return singlePod.Metadata.Annotations.CommunityComTailLogs != "false", nil
	}

	// If any pod explicitly disables logs, disable for all
	for _, podItem := range pods.Items {
		if podItem.Metadata.Annotations.CommunityComTailLogs == "false" {
			return false, nil
		}
	}

	// Default to tail
	return true, nil
}

// A StubFilter is used when we fail to talk to Kubernetes, e.g. when
// running locally.
type StubFilter struct {
	TailAll bool
}

func (f *StubFilter) ShouldTailLogs(pod *Pod) (bool, error) { return f.TailAll, nil }
