// Package client is a minimal client for the Discord REST API (v10).
//
// Shape of the API this talks to:
//   - Base URL: https://discord.com/api/v10 ; resources live under /guilds/...,
//     /channels/..., /users/..., /applications/..., /invites/..., /webhooks/...
//   - Auth: Authorization: Bot <token>. A User-Agent is required by Discord.
//   - Bodies are FLAT JSON objects (Discord is not JSON:API). A single resource
//     read returns the object directly; a collection returns a JSON array. The
//     resource id is the string snowflake in the object's "id" field.
//   - Rate limiting is real and enforced per-route: a 429 carries a JSON body
//     {"message":...,"retry_after":<seconds>,"global":<bool>}. The client honours
//     it (and retries transient 5xx) transparently, so resources never see a 429.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultEndpoint is the base URL of the Discord REST API.
const DefaultEndpoint = "https://discord.com/api/v10"

// maxAttempts bounds the retry loop for 429 / 5xx responses.
const maxAttempts = 6

// Client talks to the Discord REST API with a bot token.
type Client struct {
	httpClient     *http.Client
	endpoint       string
	token          string
	userAgent      string
	auditLogReason string

	botMu     sync.Mutex
	botUserID string
}

// New constructs a Client. An empty endpoint falls back to DefaultEndpoint. A
// non-empty auditLogReason is sent as X-Audit-Log-Reason on every mutating
// request, so changes show up attributed in the guild's audit log.
func New(endpoint, token, auditLogReason string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		endpoint:   strings.TrimRight(endpoint, "/"),
		token:      token,
		// Discord rejects requests without a descriptive User-Agent.
		userAgent:      "DiscordBot (https://github.com/kirchDev/terraform-provider-discord, 1.0)",
		auditLogReason: auditLogReason,
	}
}

// Get fetches a single resource at path and, when out is non-nil, unmarshals the
// flat JSON object into it.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// List fetches a collection at path and returns the raw JSON array elements.
// Each caller unmarshals an element into its own type.
func (c *Client) List(ctx context.Context, path string) ([]json.RawMessage, error) {
	var arr []json.RawMessage
	if err := c.do(ctx, http.MethodGet, path, nil, &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

// Write sends body as a FLAT JSON request (POST/PUT/PATCH) and, when out is
// non-nil, unmarshals the flat JSON response object into it. A nil body sends no
// payload (some Discord endpoints — e.g. add-member-role PUT — take none).
func (c *Client) Write(ctx context.Context, method, path string, body, out any) error {
	var raw []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		raw = b
	}
	return c.do(ctx, method, path, raw, out)
}

// Delete issues a DELETE against path.
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// BotUserID returns the bot's own user id (GET /users/@me), fetched once and
// cached. Used where the API treats the current user specially (e.g. changing
// the bot's own nickname goes through /members/@me, not /members/{id}).
func (c *Client) BotUserID(ctx context.Context) (string, error) {
	c.botMu.Lock()
	defer c.botMu.Unlock()
	if c.botUserID != "" {
		return c.botUserID, nil
	}
	var u struct {
		ID string `json:"id"`
	}
	if err := c.Get(ctx, "/users/@me", &u); err != nil {
		return "", err
	}
	c.botUserID = u.ID
	return u.ID, nil
}

// NotFound reports whether err is a 404 from the API (useful for Read to drop a
// resource from state when it's gone upstream).
func NotFound(err error) bool {
	var e *APIError
	if errors.As(err, &e) {
		return e.StatusCode == http.StatusNotFound
	}
	return false
}

// APIError is a non-2xx API response. Code is Discord's JSON error code (e.g.
// 10004 "Unknown Guild"), 0 when the body carried none.
type APIError struct {
	StatusCode int
	Code       int
	Method     string
	Path       string
	Body       string
}

func (e *APIError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("discord API %s %s: status %d (code %d): %s", e.Method, e.Path, e.StatusCode, e.Code, e.Body)
	}
	return fmt.Sprintf("discord API %s %s: status %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

// rateLimitBody is the shape Discord returns on a 429.
type rateLimitBody struct {
	RetryAfter float64 `json:"retry_after"`
	Global     bool    `json:"global"`
}

// errorBody carries Discord's structured error code/message on a 4xx/5xx.
type errorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// do performs an authenticated request, transparently retrying 429 (honouring
// retry_after) and transient 5xx, and decodes a 2xx JSON body into out when set.
func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	for attempt := 1; ; attempt++ {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reader)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bot "+c.token)
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		// Attribute the change in the guild audit log. Discord ignores the header
		// on GET, so only set it on mutating requests.
		if c.auditLogReason != "" && method != http.MethodGet {
			req.Header.Set("X-Audit-Log-Reason", c.auditLogReason)
		}

		res, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if readErr != nil {
			return readErr
		}

		// Rate limited: wait out retry_after (body is most precise; fall back to
		// the Retry-After header) and try again.
		if res.StatusCode == http.StatusTooManyRequests && attempt < maxAttempts {
			if err := sleepCtx(ctx, retryAfter(res, data)); err != nil {
				return err
			}
			continue
		}

		// Transient server errors: exponential backoff and retry.
		if res.StatusCode >= 500 && attempt < maxAttempts {
			if err := sleepCtx(ctx, backoff(attempt)); err != nil {
				return err
			}
			continue
		}

		if res.StatusCode < 200 || res.StatusCode >= 300 {
			apiErr := &APIError{StatusCode: res.StatusCode, Method: method, Path: path, Body: strings.TrimSpace(string(data))}
			var eb errorBody
			if json.Unmarshal(data, &eb) == nil {
				apiErr.Code = eb.Code
			}
			return apiErr
		}

		if out != nil && len(data) > 0 {
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("decoding %s %s response: %w", method, path, err)
			}
		}
		return nil
	}
}

// retryAfter derives how long to wait before retrying a 429.
func retryAfter(res *http.Response, data []byte) time.Duration {
	var rl rateLimitBody
	if json.Unmarshal(data, &rl) == nil && rl.RetryAfter > 0 {
		return time.Duration(rl.RetryAfter * float64(time.Second))
	}
	if h := res.Header.Get("Retry-After"); h != "" {
		if secs, err := strconv.ParseFloat(h, 64); err == nil && secs > 0 {
			return time.Duration(secs * float64(time.Second))
		}
	}
	return time.Second
}

// backoff returns the wait before the attempt-th retry of a 5xx (1s, 2s, 4s, …,
// capped at 30s).
func backoff(attempt int) time.Duration {
	d := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// sleepCtx waits for d, returning early if ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
