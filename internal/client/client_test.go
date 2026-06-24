package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// recorded captures what the test server saw on the last request, so a test can
// assert on the method, path, headers and (flat JSON) body the client sent.
type recorded struct {
	method      string
	path        string
	auth        string
	ua          string
	accept      string
	ctype       string
	auditReason string
	body        []byte
}

func newServer(t *testing.T, rec *recorded, respond func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec.method = r.Method
		rec.path = r.URL.Path
		rec.auth = r.Header.Get("Authorization")
		rec.ua = r.Header.Get("User-Agent")
		rec.accept = r.Header.Get("Accept")
		rec.ctype = r.Header.Get("Content-Type")
		rec.auditReason = r.Header.Get("X-Audit-Log-Reason")
		rec.body = body
		respond(w, r)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "secret-token", "managed by tofu")
}

func TestNewDefaultsEndpoint(t *testing.T) {
	if c := New("", "tok", ""); c.endpoint != DefaultEndpoint {
		t.Errorf("empty endpoint = %q, want %q", c.endpoint, DefaultEndpoint)
	}
	if got := New("https://example.test/", "tok", "").endpoint; got != "https://example.test" {
		t.Errorf("trailing slash not trimmed: %q", got)
	}
}

func TestAuditLogReasonHeader(t *testing.T) {
	// Mutating requests carry X-Audit-Log-Reason; GET does not.
	var rec recorded
	c := newServer(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"1"}`)
	})

	if err := c.Write(context.Background(), http.MethodPatch, "/channels/1", map[string]any{"name": "x"}, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if rec.auditReason != "managed by tofu" {
		t.Errorf("PATCH audit reason = %q, want %q", rec.auditReason, "managed by tofu")
	}

	if err := c.Get(context.Background(), "/channels/1", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.auditReason != "" {
		t.Errorf("GET should not carry an audit reason, got %q", rec.auditReason)
	}
}

func TestGetParsesFlatJSONAndSetsHeaders(t *testing.T) {
	var rec recorded
	c := newServer(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Discord returns a FLAT object — the id is the string snowflake in "id".
		_, _ = io.WriteString(w, `{"id":"123","name":"general","position":2}`)
	})

	var ch struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Position int64  `json:"position"`
	}
	if err := c.Get(context.Background(), "/channels/123", &ch); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ch.ID != "123" || ch.Name != "general" || ch.Position != 2 {
		t.Errorf("decoded wrong: %+v", ch)
	}
	if rec.method != http.MethodGet || rec.path != "/channels/123" {
		t.Errorf("method/path = %q %q", rec.method, rec.path)
	}
	if rec.auth != "Bot secret-token" {
		t.Errorf("auth header = %q, want Bot prefix", rec.auth)
	}
	if rec.ua == "" {
		t.Error("User-Agent header missing (Discord requires it)")
	}
	if rec.accept != "application/json" {
		t.Errorf("accept = %q", rec.accept)
	}
}

func TestListReturnsRawArray(t *testing.T) {
	var rec recorded
	c := newServer(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"id":"1","name":"a"},{"id":"2","name":"b"}]`)
	})

	res, err := c.List(context.Background(), "/guilds/9/roles")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("len = %d, want 2", len(res))
	}
	var role struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(res[1], &role); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if role.ID != "2" || role.Name != "b" {
		t.Errorf("element wrong: %+v", role)
	}
}

func TestWriteSendsFlatBody(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			var rec recorded
			c := newServer(t, &rec, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, `{"id":"7","name":"new"}`)
			})

			body := map[string]any{"name": "new", "color": 5814783}
			var out struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if err := c.Write(context.Background(), method, "/guilds/9/roles", body, &out); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if out.ID != "7" || out.Name != "new" {
				t.Errorf("decoded wrong: %+v", out)
			}
			if rec.method != method {
				t.Errorf("method = %q, want %q", rec.method, method)
			}
			if rec.ctype != "application/json" {
				t.Errorf("content-type = %q", rec.ctype)
			}
			var sent map[string]any
			if err := json.Unmarshal(rec.body, &sent); err != nil {
				t.Fatalf("sent body not JSON: %v (%s)", err, rec.body)
			}
			if sent["name"] != "new" {
				t.Errorf("flat body fields wrong: %s", rec.body)
			}
		})
	}
}

func TestWriteNilBodySendsNoPayload(t *testing.T) {
	var rec recorded
	c := newServer(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.Write(context.Background(), http.MethodPut, "/guilds/9/members/1/roles/2", nil, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(rec.body) != 0 {
		t.Errorf("nil body should send empty payload, got %q", rec.body)
	}
	if rec.ctype != "" {
		t.Errorf("nil body should not set Content-Type, got %q", rec.ctype)
	}
}

func TestDeleteIssuesDelete(t *testing.T) {
	var rec recorded
	c := newServer(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.Delete(context.Background(), "/channels/7"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if rec.method != http.MethodDelete || rec.path != "/channels/7" {
		t.Errorf("method/path = %q %q", rec.method, rec.path)
	}
}

func TestNotFoundAndErrorCode(t *testing.T) {
	c := newServer(t, &recorded{}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Unknown Guild","code":10004}`)
	})
	err := c.Get(context.Background(), "/guilds/999", nil)
	if err == nil || !NotFound(err) {
		t.Fatalf("expected NotFound, got %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 10004 {
		t.Errorf("expected APIError code 10004, got %v", err)
	}

	c2 := newServer(t, &recorded{}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"Missing Permissions","code":50013}`)
	})
	err = c2.Write(context.Background(), http.MethodPatch, "/channels/1", map[string]any{}, nil)
	if NotFound(err) {
		t.Error("403 should not be NotFound")
	}
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || apiErr.Code != 50013 {
		t.Errorf("expected 403 code 50013, got %v", err)
	}
}

func TestWriteMultipart(t *testing.T) {
	// The file part must carry the real Content-Type (image/png), not the
	// multipart default (octet-stream) — Discord rejects the latter as Invalid Asset.
	var rec recorded
	c := newServer(t, &rec, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"9"}`)
	})

	var out struct {
		ID string `json:"id"`
	}
	err := c.WriteMultipart(context.Background(), http.MethodPost, "/guilds/1/stickers",
		map[string]string{"name": "starbear", "tags": "test"}, "file", "sticker.png", "image/png", []byte("PNGDATA"), &out)
	if err != nil {
		t.Fatalf("WriteMultipart: %v", err)
	}
	if out.ID != "9" {
		t.Errorf("id = %q, want 9", out.ID)
	}
	if !strings.HasPrefix(rec.ctype, "multipart/form-data") {
		t.Errorf("content-type = %q, want multipart/form-data", rec.ctype)
	}
	body := string(rec.body)
	for _, want := range []string{`name="file"; filename="sticker.png"`, "Content-Type: image/png", "PNGDATA", `name="name"`, "starbear"} {
		if !strings.Contains(body, want) {
			t.Errorf("multipart body missing %q:\n%s", want, body)
		}
	}
}

func TestRetriesOn429(t *testing.T) {
	var calls atomic.Int32
	c := newServer(t, &recorded{}, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			// First call: rate limited with a tiny retry_after so the test is fast.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"message":"rate limited","retry_after":0.01,"global":false}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"1"}`)
	})

	var out struct {
		ID string `json:"id"`
	}
	if err := c.Get(context.Background(), "/guilds/1", &out); err != nil {
		t.Fatalf("Get with retry: %v", err)
	}
	if out.ID != "1" {
		t.Errorf("decoded wrong after retry: %+v", out)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("expected 2 calls (429 then 200), got %d", got)
	}
}
