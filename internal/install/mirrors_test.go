package install

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/goodtiger/openclaw-install/presets"
)

func TestResolveMirrorsPrefersReachableCandidate(t *testing.T) {
	workflow := NewWorkflow(presets.Bundle{
		Mirrors: presets.MirrorManifest{
			Categories: map[string][]presets.MirrorCandidate{
				"npm_registry": {
					{Name: "offline", BaseURL: "https://offline.example", ProbeURL: "http://127.0.0.1:1"},
					{Name: "reachable", BaseURL: "https://reachable.example", ProbeURL: "https://reachable.example/ping"},
				},
				"github_release": {
					{Name: "fallback", BaseURL: "https://fallback.example", ProbeURL: "http://127.0.0.1:1"},
				},
			},
		},
	}, nil)
	workflow.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == "https://reachable.example/ping" {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("ok")),
					Header:     make(http.Header),
				}, nil
			}
			return nil, context.DeadlineExceeded
		}),
	}

	selection, warnings := workflow.ResolveMirrors(context.Background())

	if got := selection["npm_registry"].Name; got != "reachable" {
		t.Fatalf("expected reachable mirror, got %q", got)
	}

	if got := selection["github_release"].Name; got != "fallback" {
		t.Fatalf("expected fallback mirror, got %q", got)
	}

	if len(warnings) == 0 {
		t.Fatal("expected at least one warning for fallback mirror selection")
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
