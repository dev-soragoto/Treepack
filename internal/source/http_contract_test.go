package source

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDoWithRetryRetriesTransportErrorThenSucceeds(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("temporary transport failure")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})}

	resp, gotAttempts, err := doWithRetry(client, http.MethodGet, "https://example.test/asset", nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if attempts != 2 || gotAttempts != 2 {
		t.Fatalf("transport attempts = %d, returned attempts = %d; want 2, 2", attempts, gotAttempts)
	}
}

func TestDoWithRetryReportsExhaustedTransportErrors(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return nil, errors.New("network unavailable")
	})}

	resp, gotAttempts, err := doWithRetry(client, http.MethodGet, "https://downloads.example.test/asset", nil, 3)
	if resp != nil || err == nil {
		t.Fatalf("response = %#v, error = %v; want nil response and error", resp, err)
	}
	if attempts != 3 || gotAttempts != 3 {
		t.Fatalf("transport attempts = %d, returned attempts = %d; want 3, 3", attempts, gotAttempts)
	}
	for _, want := range []string{"downloads.example.test", "after 3 attempt(s)", "network unavailable"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}

func TestHeadersForURLLimitsGitHubTokenHosts(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		token string
		want  bool
	}{
		{name: "github", url: "https://github.com/owner/repo/releases/download/v1/a.zip", token: "secret", want: true},
		{name: "github objects", url: "https://objects.githubusercontent.com/object", token: "secret", want: true},
		{name: "github releases", url: "https://github-releases.githubusercontent.com/object", token: "secret", want: true},
		{name: "ordinary domain", url: "https://example.com/a.zip", token: "secret"},
		{name: "spoofed suffix", url: "https://github.com.attacker.example/a.zip", token: "secret"},
		{name: "spoofed prefix", url: "https://objects.githubusercontent.com.attacker.example/a.zip", token: "secret"},
		{name: "invalid URL", url: "://not-a-url", token: "secret"},
		{name: "empty token", url: "https://github.com/owner/repo/a.zip"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			headers := headersForURL(tc.url, tc.token)
			got, present := headers["Authorization"]
			if present != tc.want {
				t.Fatalf("Authorization present = %v, want %v; headers = %#v", present, tc.want, headers)
			}
			if tc.want && got != "Bearer "+tc.token {
				t.Fatalf("Authorization = %q, want Bearer token", got)
			}
		})
	}
}
