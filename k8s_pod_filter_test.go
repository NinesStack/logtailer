package main

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jarcoal/httpmock"
	. "github.com/smartystreets/goconvey/convey"
)

var credsPath = "fixtures/discovery"

func Test_NewAnnotationPodFilter(t *testing.T) {
	Convey("NewAnnotationPodFilter()", t, func() {

		Convey("returns a properly configured struct", func() {
			filter := NewAnnotationPodFilter("beowulf.example.com", 443, 10*time.Millisecond, credsPath)

			So(filter, ShouldNotBeNil)
			So(filter.Timeout, ShouldEqual, 10*time.Millisecond)
			So(filter.KubeHost, ShouldEqual, "beowulf.example.com")
			So(filter.KubePort, ShouldEqual, 443)
			So(filter.token, ShouldContainSubstring, "this would be a token")
			So(filter.client, ShouldNotBeNil)
		})

		Convey("logs when it can't read the token", func() {
			var filter *AnnotationPodFilter

			capture := LogCapture(func() {
				filter = NewAnnotationPodFilter("beowulf.example.com", 443, 10*time.Millisecond, "/tmp/does-not-exist")
			})

			So(filter, ShouldBeNil)
			So(capture, ShouldContainSubstring, "Failed to read serviceaccount token")
		})

		Convey("logs when it can't read the CA.crt", func() {
			var filter *AnnotationPodFilter

			capture := LogCapture(func() {
				filter = NewAnnotationPodFilter("beowulf.example.com", 443, 10*time.Millisecond, credsPath+"/bad-fixture")
			})

			So(filter, ShouldNotBeNil)
			So(capture, ShouldContainSubstring, "No certs appended!")

			So(filter.Timeout, ShouldEqual, 10*time.Millisecond)
			So(filter.KubeHost, ShouldEqual, "beowulf.example.com")
			So(filter.KubePort, ShouldEqual, 443)
			So(filter.token, ShouldContainSubstring, "this would be a token")
			So(filter.client, ShouldNotBeNil)
		})
	})
}

func Test_makeRequest(t *testing.T) {
	Convey("makeRequest()", t, func() {
		Reset(func() { httpmock.DeactivateAndReset() })

		filter := NewAnnotationPodFilter("beowulf.example.com", 80, 10*time.Millisecond, credsPath)
		httpmock.ActivateNonDefault(filter.client)

		Convey("makes a request with the right headers and auth", func() {
			var auth string
			httpmock.RegisterResponder("GET", "http://beowulf.example.com:80/nowhere",
				func(req *http.Request) (*http.Response, error) {
					auth = req.Header.Get("Authorization")
					return httpmock.NewJsonResponse(200, map[string]interface{}{"success": "yeah"})
				},
			)

			body, err := filter.makeRequest("/nowhere")
			So(err, ShouldBeNil)
			So(auth, ShouldStartWith, "Bearer ")
			So(auth, ShouldContainSubstring, "this would be a token")

			So(body, ShouldNotBeEmpty)
		})

		Convey("handles non-200 status code", func() {
			var auth string
			httpmock.RegisterResponder("GET", "http://beowulf.example.com:80/nowhere",
				func(req *http.Request) (*http.Response, error) {
					auth = req.Header.Get("Authorization")
					return httpmock.NewJsonResponse(403, map[string]interface{}{"bad": "times"})
				},
			)

			body, err := filter.makeRequest("/nowhere")
			So(err, ShouldNotBeNil)
			So(auth, ShouldStartWith, "Bearer ")
			So(auth, ShouldContainSubstring, "this would be a token")

			So(err.Error(), ShouldContainSubstring, "got unexpected response code from /nowhere: 403")
			So(body, ShouldBeEmpty)
		})

		Convey("handles error back from http call", func() {
			httpmock.RegisterResponder("GET", "http://beowulf.example.com:80/nowhere",
				httpmock.NewErrorResponder(errors.New("intentional test error")),
			)

			body, err := filter.makeRequest("/nowhere")

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "intentional test error")
			So(body, ShouldBeEmpty)
		})
	})
}

func Test_ShouldTailLogs(t *testing.T) {
	Convey("ShouldTailLogs()", t, func() {
		Reset(func() { httpmock.DeactivateAndReset() })

		filter := NewAnnotationPodFilter("beowulf.example.com", 80, 10*time.Millisecond, credsPath)
		httpmock.ActivateNonDefault(filter.client)

		Convey("makes a request with the right headers and auth", func() {
			var auth string
			httpmock.RegisterResponder("GET", "=~http://beowulf.example.com:80/api/v1/namespaces/the-awesome-place/pods.*",
				func(req *http.Request) (*http.Response, error) {
					auth = req.Header.Get("Authorization")
					// We need to return more than pod here
					return httpmock.NewStringResponse(200, `{"items":[{"metadata":{"annotations":{}}},{"metadata":{"annotations": {"community.com/TailLogs":"true"}}}]}`), nil
				},
			)

			pod := &Pod{
				Name:        "awesome-pod",
				ServiceName: "awesome-pod",
				Namespace:   "the-awesome-place",
			}

			shouldTail, err := filter.ShouldTailLogs(pod)
			So(err, ShouldBeNil)
			So(auth, ShouldStartWith, "Bearer ")
			So(auth, ShouldContainSubstring, "this would be a token")

			So(shouldTail, ShouldBeTrue)
		})

		Convey("returns false when annotation is not set", func() {
			httpmock.RegisterResponder("GET", "=~http://beowulf.example.com:80/api/v1/namespaces/the-awesome-place/pods.*",
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewStringResponse(200, `{"items":[{"metadata":{"annotations":{}}}]}`), nil
				},
			)

			pod := &Pod{
				Name:        "awesome-pod",
				ServiceName: "awesome-pod",
				Namespace:   "the-awesome-place",
			}

			shouldTail, err := filter.ShouldTailLogs(pod)
			So(err, ShouldBeNil)
			So(shouldTail, ShouldBeFalse)
		})
	})
}

func Test_NewTailAllFilter(t *testing.T) {
	Convey("NewTailAllFilter()", t, func() {

		Convey("returns a properly configured struct", func() {
			filter := NewTailAllFilter("beowulf.example.com", 443, 10*time.Millisecond, credsPath)

			So(filter, ShouldNotBeNil)
			So(filter.AnnotationPodFilter, ShouldNotBeNil)
			So(filter.AnnotationPodFilter.Timeout, ShouldEqual, 10*time.Millisecond)
			So(filter.AnnotationPodFilter.KubeHost, ShouldEqual, "beowulf.example.com")
			So(filter.AnnotationPodFilter.KubePort, ShouldEqual, 443)
		})

		Convey("returns nil when underlying filter fails", func() {
			filter := NewTailAllFilter("beowulf.example.com", 443, 10*time.Millisecond, "/tmp/does-not-exist")

			So(filter, ShouldBeNil)
		})
	})
}

func Test_TailAllFilter_ShouldTailLogs(t *testing.T) {
	Convey("TailAllFilter.ShouldTailLogs()", t, func() {
		Reset(func() { httpmock.DeactivateAndReset() })

		filter := NewTailAllFilter("beowulf.example.com", 80, 10*time.Millisecond, credsPath)
		httpmock.ActivateNonDefault(filter.client)

		Convey("returns true by default when no annotation is set", func() {
			httpmock.RegisterResponder("GET", "=~http://beowulf.example.com:80/api/v1/namespaces/the-awesome-place/pods.*",
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewStringResponse(200, `{"items":[{"metadata":{"annotations":{}}}]}`), nil
				},
			)

			pod := &Pod{
				Name:        "awesome-pod",
				ServiceName: "awesome-pod",
				Namespace:   "the-awesome-place",
			}

			shouldTail, err := filter.ShouldTailLogs(pod)
			So(err, ShouldBeNil)
			So(shouldTail, ShouldBeTrue)
		})

		Convey("respects community.com/TailLogs=false annotation", func() {
			httpmock.RegisterResponder("GET", "=~http://beowulf.example.com:80/api/v1/namespaces/the-awesome-place/pods.*",
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewStringResponse(200, `{"items":[{"metadata":{"annotations":{"community.com/TailLogs":"false"}}}]}`), nil
				},
			)

			pod := &Pod{
				Name:        "awesome-pod",
				ServiceName: "awesome-pod",
				Namespace:   "the-awesome-place",
			}

			shouldTail, err := filter.ShouldTailLogs(pod)
			So(err, ShouldBeNil)
			So(shouldTail, ShouldBeFalse)
		})

		Convey("ignores community.com/TailLogs=true annotation (still returns true)", func() {
			httpmock.RegisterResponder("GET", "=~http://beowulf.example.com:80/api/v1/namespaces/the-awesome-place/pods.*",
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewStringResponse(200, `{"items":[{"metadata":{"annotations":{"community.com/TailLogs":"true"}}}]}`), nil
				},
			)

			pod := &Pod{
				Name:        "awesome-pod",
				ServiceName: "awesome-pod",
				Namespace:   "the-awesome-place",
			}

			shouldTail, err := filter.ShouldTailLogs(pod)
			So(err, ShouldBeNil)
			So(shouldTail, ShouldBeTrue)
		})

		Convey("returns true when no pods found (unknown pod)", func() {
			httpmock.RegisterResponder("GET", "=~http://beowulf.example.com:80/api/v1/namespaces/the-awesome-place/pods.*",
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewStringResponse(200, `{"items":[]}`), nil
				},
			)

			pod := &Pod{
				Name:        "unknown-pod",
				ServiceName: "unknown-pod",
				Namespace:   "the-awesome-place",
			}

			shouldTail, err := filter.ShouldTailLogs(pod)
			So(err, ShouldBeNil)
			So(shouldTail, ShouldBeTrue)
		})
	})
}

func Test_DiscoveryFilter_Interface(t *testing.T) {
	Convey("DiscoveryFilter interface", t, func() {

		Convey("AnnotationPodFilter implements DiscoveryFilter", func() {
			var filter DiscoveryFilter
			filter = NewAnnotationPodFilter("beowulf.example.com", 443, 10*time.Millisecond, credsPath)

			So(filter, ShouldNotBeNil)
			So(filter, ShouldImplement, (*DiscoveryFilter)(nil))
		})

		Convey("TailAllFilter implements DiscoveryFilter", func() {
			var filter DiscoveryFilter
			filter = NewTailAllFilter("beowulf.example.com", 443, 10*time.Millisecond, credsPath)

			So(filter, ShouldNotBeNil)
			So(filter, ShouldImplement, (*DiscoveryFilter)(nil))
		})

		Convey("StubFilter implements DiscoveryFilter", func() {
			var filter DiscoveryFilter
			filter = &StubFilter{TailAll: false}

			So(filter, ShouldNotBeNil)
			So(filter, ShouldImplement, (*DiscoveryFilter)(nil))
		})
	})
}
