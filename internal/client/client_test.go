package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestErrorEnvelopeAndRetries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer tr_v2_key" {
			t.Errorf("missing bearer token")
		}
		switch r.URL.Path {
		case "/v1/flaky":
			if n < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"id":"ok"}`))
		case "/v1/create":
			w.WriteHeader(http.StatusBadGateway)
		default:
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_request","message":"rotations.0.cadence: bad","details":{"issues":[{"path":"rotations.0.cadence","message":"bad"}]}}}`))
		}
	}))
	defer server.Close()
	c := New(server.URL, "tr_v2_key", "test")
	c.Backoff = 0

	var out struct{ ID string }
	if err := c.Do(context.Background(), http.MethodGet, "/v1/flaky", nil, &out); err != nil || out.ID != "ok" {
		t.Fatalf("GET should retry 503 until success: %v %v", err, out)
	}
	calls.Store(0)
	if err := c.Do(context.Background(), http.MethodPost, "/v1/create", map[string]string{}, nil); err == nil || calls.Load() != 1 {
		t.Fatalf("POST must not retry a 502 (it may have created): calls=%d err=%v", calls.Load(), err)
	}
	err := c.Do(context.Background(), http.MethodPut, "/v1/invalid", map[string]string{}, nil)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != 422 || apiErr.Code != "invalid_request" || len(apiErr.Details.Issues) != 1 {
		t.Fatalf("unexpected error %#v", err)
	}
	if IsNotFound(err) {
		t.Fatal("422 is not a 404")
	}
}

func TestPathEscapesSegments(t *testing.T) {
	if got := OnCall("a b", "schedules", "s/1"); got != "/v1/on-call/configurations/a%20b/schedules/s%2F1" {
		t.Fatal(got)
	}
}
