// Package client is a small HTTP client for the Transmit on-call API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.transmit.dev"

// Client calls the Transmit public API with one API key.
type Client struct {
	BaseURL   string
	APIKey    string
	HTTP      *http.Client
	UserAgent string
	// Backoff between retries of a retryable response; tests shorten it.
	Backoff time.Duration
}

func New(baseURL, apiKey, userAgent string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, HTTP: &http.Client{Timeout: 60 * time.Second}, UserAgent: userAgent, Backoff: time.Second}
}

// APIError is the API's error envelope.
type APIError struct {
	Status  int
	Code    string `json:"code"`
	Message string `json:"message"`
	Details *struct {
		Issues []struct {
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"issues"`
	} `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("transmit API returned HTTP %d", e.Status)
	}
	return fmt.Sprintf("%s (HTTP %d, %s)", e.Message, e.Status, e.Code)
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

func retryable(method string, status int) bool {
	// POST creates; retrying it after an ambiguous failure could create twice.
	if method == http.MethodPost {
		return status == http.StatusTooManyRequests
	}
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

// Do sends one request and decodes a JSON response into out (when out is not nil).
func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var payload []byte
	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return err
		}
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.Backoff * time.Duration(1<<(attempt-1))):
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Accept", "application/json")
		if c.UserAgent != "" {
			req.Header.Set("User-Agent", c.UserAgent)
		}
		if in != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			if out == nil || len(body) == 0 {
				return nil
			}
			return json.Unmarshal(body, out)
		}
		apiErr := &APIError{Status: res.StatusCode}
		var envelope struct {
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal(body, &envelope) == nil && len(envelope.Error) > 0 {
			if json.Unmarshal(envelope.Error, apiErr) != nil {
				// Older endpoints return {"error": "message", "code": "..."}.
				var flat struct {
					Error string `json:"error"`
					Code  string `json:"code"`
				}
				_ = json.Unmarshal(body, &flat)
				apiErr.Message, apiErr.Code = flat.Error, flat.Code
			}
			apiErr.Status = res.StatusCode
		}
		lastErr = apiErr
		if !retryable(method, res.StatusCode) {
			return apiErr
		}
	}
	return lastErr
}

// Path joins escaped segments under /v1.
func Path(segments ...string) string {
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = url.PathEscape(segment)
	}
	return "/v1/" + strings.Join(escaped, "/")
}

func OnCall(configurationID string, segments ...string) string {
	return Path(append([]string{"on-call", "configurations", configurationID}, segments...)...)
}

/* ---------------------------------------------------------------- types */

type Configuration struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Revision      int64  `json:"revision"`
	PagingEnabled bool   `json:"paging_enabled"`
	Imported      bool   `json:"imported"`
	CreatedAt     string `json:"created_at"`
}

type Responder struct {
	ID              string `json:"id"`
	ConfigurationID string `json:"configuration_id"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	Disabled        bool   `json:"disabled"`
	ContactVerified bool   `json:"contact_verified"`
}
type ResponderInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Disabled bool   `json:"disabled"`
}

type Team struct {
	ID              string   `json:"id"`
	ConfigurationID string   `json:"configuration_id"`
	Name            string   `json:"name"`
	ResponderIDs    []string `json:"responder_ids"`
}
type TeamInput struct {
	Name         string   `json:"name"`
	ResponderIDs []string `json:"responderIds"`
}

type Rotation struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Cadence      string   `json:"cadence"`
	Interval     int64    `json:"interval"`
	StartsAt     string   `json:"starts_at"`
	EndsAt       *string  `json:"ends_at"`
	ResponderIDs []string `json:"responder_ids"`
}
type Schedule struct {
	ID              string     `json:"id"`
	ConfigurationID string     `json:"configuration_id"`
	Name            string     `json:"name"`
	Timezone        string     `json:"timezone"`
	Enabled         bool       `json:"enabled"`
	OwnerTeamID     *string    `json:"owner_team_id"`
	Rotations       []Rotation `json:"rotations"`
}
type RotationInput struct {
	Name         string   `json:"name"`
	Cadence      string   `json:"cadence"`
	Interval     int64    `json:"interval"`
	StartsAt     string   `json:"startsAt"`
	EndsAt       *string  `json:"endsAt"`
	ResponderIDs []string `json:"responderIds"`
}
type ScheduleInput struct {
	Name        string          `json:"name"`
	Timezone    string          `json:"timezone"`
	Enabled     bool            `json:"enabled"`
	OwnerTeamID *string         `json:"ownerTeamId"`
	Rotations   []RotationInput `json:"rotations"`
}

type Override struct {
	ID              string   `json:"id"`
	ScheduleID      string   `json:"schedule_id"`
	ConfigurationID string   `json:"configuration_id"`
	StartsAt        string   `json:"starts_at"`
	EndsAt          string   `json:"ends_at"`
	ResponderID     string   `json:"responder_id"`
	RotationIDs     []string `json:"rotation_ids"`
}
type OverrideInput struct {
	StartsAt    string   `json:"startsAt"`
	EndsAt      string   `json:"endsAt"`
	ResponderID string   `json:"responderId"`
	RotationIDs []string `json:"rotationIds,omitempty"`
}

type Recipient struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type Step struct {
	DelaySeconds int64     `json:"delay_seconds"`
	Recipient    Recipient `json:"recipient"`
}
type EscalationPolicy struct {
	ID              string  `json:"id"`
	ConfigurationID string  `json:"configuration_id"`
	Name            string  `json:"name"`
	OwnerTeamID     *string `json:"owner_team_id"`
	Steps           []Step  `json:"steps"`
}
type StepInput struct {
	DelaySeconds int64     `json:"delaySeconds"`
	Recipient    Recipient `json:"recipient"`
}
type EscalationPolicyInput struct {
	Name        string      `json:"name"`
	OwnerTeamID *string     `json:"ownerTeamId"`
	Steps       []StepInput `json:"steps"`
}

type Heartbeat struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	ConfigurationID    string  `json:"configuration_id"`
	EscalationPolicyID string  `json:"escalation_policy_id"`
	IntervalSeconds    int64   `json:"interval_seconds"`
	GraceSeconds       int64   `json:"grace_seconds"`
	Sandbox            bool    `json:"sandbox"`
	State              string  `json:"state"`
	LastPingAt         *string `json:"last_ping_at"`
	DueAt              *string `json:"due_at"`
	IncidentID         *string `json:"incident_id"`
	PingURL            string  `json:"ping_url"`
}
type HeartbeatCreate struct {
	Name               string `json:"name"`
	ConfigurationID    string `json:"configurationId"`
	EscalationPolicyID string `json:"escalationPolicyId"`
	IntervalSeconds    int64  `json:"intervalSeconds"`
	GraceSeconds       int64  `json:"graceSeconds"`
}
type HeartbeatUpdate struct {
	Name               string `json:"name"`
	EscalationPolicyID string `json:"escalationPolicyId"`
	IntervalSeconds    int64  `json:"intervalSeconds"`
	GraceSeconds       int64  `json:"graceSeconds"`
}

type List[T any] struct {
	Data []T `json:"data"`
}
