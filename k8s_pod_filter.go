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
	loghttp "github.com/motemen/go-loghttp"
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

// A PodFilter calls out to the Kubernetes API and determines if annotations
// are present on a pod that would enable us to track logs for that pod.
type PodFilter struct {
	Timeout time.Duration

	KubeHost string
	KubePort int

	token  string
	client *http.Client
}

func NewPodFilter(kubeHost string, kubePort int, timeout time.Duration, credsPath string) *PodFilter {
	f := &PodFilter{
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

	transport := &loghttp.Transport{
		LogRequest: func(req *http.Request) {
			log.Printf("[%#v] %s %#v", req, req.Method, req.URL)
		},
		LogResponse: func(resp *http.Response) {
			log.Printf("[%#v] %d %#v", resp.Request, resp.StatusCode, resp.Request.URL)
		},
		Transport: &http.Transport{TLSClientConfig: config},
	}

	f.client.Transport = transport

	return f
}

func (f *PodFilter) makeRequest(path string) ([]byte, error) {
	var scheme = "http"
	if f.KubePort == 443 {
		scheme = "https"
	}

	apiURL := url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", f.KubeHost, f.KubePort),
		Path:   path,
	}

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

func (f *PodFilter) ShouldTailLogs(pod *Pod) (bool, error) {
	body, err := f.makeRequest(
		"/api/v1/namespaces/" + pod.Namespace + "/pods?limit=100&labelSelector=ServiceName%3D" + pod.ServiceName,
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
	return (pods.Items[0].Metadata.Annotations.CommunityComTailLogs == "true"), nil
}

// A StubFilter is used when we fail to talk to Kubernetes, e.g. when
// running locally.
type StubFilter struct{}

func (f *StubFilter) ShouldTailLogs(pod *Pod) (bool, error) { return true, nil }
