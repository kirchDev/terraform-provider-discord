package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testAccProtoV6ProviderFactories wires the in-process provider for the
// terraform-plugin-testing harness. Tests point it at a mockDiscord via
// DISCORD_ENDPOINT, so the full plan/apply/refresh/import/destroy cycle runs
// against an in-memory API — no token, no real Discord.
func testAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"discord": providerserver.NewProtocol6WithError(New("test")()),
	}
}

// mockDiscord is an in-memory stand-in for the slice of the Discord REST API the
// representative resource tests touch: guild roles (a sub-collection, with the
// separate modify-positions PATCH) and channels (created under a guild, then
// addressed globally by id). It mirrors the real contract: flat JSON bodies,
// flat JSON responses, string snowflake ids.
type mockDiscord struct {
	url string

	mu       sync.Mutex
	nextID   int
	roles    map[string]map[string]any // role id -> attrs
	channels map[string]map[string]any // channel id -> attrs
	guilds   map[string]map[string]any // guild id -> attrs
	members  map[string]map[string]any // "guildID/userID" -> attrs
	automod  map[string]map[string]any // rule id -> attrs
}

func newMockDiscord(t *testing.T) *mockDiscord {
	t.Helper()
	m := &mockDiscord{
		roles:    map[string]map[string]any{},
		channels: map[string]map[string]any{},
		guilds:   map[string]map[string]any{},
		members:  map[string]map[string]any{},
		automod:  map[string]map[string]any{},
	}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	m.url = srv.URL

	t.Setenv("DISCORD_ENDPOINT", m.url)
	t.Setenv("DISCORD_TOKEN", "test")

	// The harness reattaches the provider under the address tofu resolves a bare
	// "discord" to; the framework otherwise defaults to a registry.terraform.io
	// address tofu rejects.
	t.Setenv("TF_ACC_PROVIDER_HOST", "registry.opentofu.org")
	t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "hashicorp")

	if os.Getenv("TF_ACC_TERRAFORM_PATH") == "" {
		if tofu, err := exec.LookPath("tofu"); err == nil {
			t.Setenv("TF_ACC_TERRAFORM_PATH", tofu)
		}
	}
	return m
}

func (m *mockDiscord) id() string {
	m.nextID++
	return strconv.Itoa(100000000000000000 + m.nextID)
}

func (m *mockDiscord) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	segs := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(segs) == 3 && segs[0] == "guilds" && segs[2] == "roles":
		m.serveRolesCollection(w, r, segs[1])
	case len(segs) == 4 && segs[0] == "guilds" && segs[2] == "roles":
		m.serveRoleItem(w, r, segs[3])
	case len(segs) == 3 && segs[0] == "guilds" && segs[2] == "channels":
		m.serveChannelsCollection(w, r, segs[1])
	case len(segs) == 2 && segs[0] == "guilds":
		m.serveGuildItem(w, r, segs[1])
	case len(segs) == 4 && segs[0] == "guilds" && segs[2] == "members":
		m.serveMemberItem(w, r, segs[1]+"/"+segs[3])
	case len(segs) == 4 && segs[0] == "guilds" && segs[2] == "auto-moderation" && segs[3] == "rules":
		m.serveAutomodCollection(w, r, segs[1])
	case len(segs) == 5 && segs[0] == "guilds" && segs[2] == "auto-moderation" && segs[3] == "rules":
		m.serveAutomodItem(w, r, segs[1], segs[4])
	case len(segs) == 4 && segs[0] == "channels" && segs[2] == "permissions":
		m.serveChannelPermission(w, r, segs[1], segs[3])
	case len(segs) == 2 && segs[0] == "channels":
		m.serveChannelItem(w, r, segs[1])
	default:
		http.Error(w, `{"message":"not found","code":0}`, http.StatusNotFound)
	}
}

// serveGuildItem adopts-and-reads a guild (auto-initialised with defaults so a
// manage-not-create resource can PATCH then read it).
func (m *mockDiscord) serveGuildItem(w http.ResponseWriter, r *http.Request, id string) {
	a, ok := m.guilds[id]
	if !ok {
		a = map[string]any{
			"id": id, "name": "Test Guild", "owner_id": "111111111111111111",
			"afk_timeout": float64(300), "default_message_notifications": float64(0),
			"explicit_content_filter": float64(0), "verification_level": float64(0),
			"preferred_locale": "en-US", "premium_progress_bar_enabled": false,
		}
		m.guilds[id] = a
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, a)
	case http.MethodPatch:
		for k, v := range decodeObject(r) {
			a[k] = v
		}
		writeJSON(w, http.StatusOK, a)
	default:
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// serveMemberItem reads/updates a guild member (auto-initialised with an empty
// role set).
func (m *mockDiscord) serveMemberItem(w http.ResponseWriter, r *http.Request, key string) {
	a, ok := m.members[key]
	if !ok {
		a = map[string]any{"roles": []any{}, "user": map[string]any{"id": "555", "username": "tester"}}
		m.members[key] = a
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, a)
	case http.MethodPatch:
		for k, v := range decodeObject(r) {
			a[k] = v
		}
		writeJSON(w, http.StatusOK, a)
	default:
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (m *mockDiscord) serveAutomodCollection(w http.ResponseWriter, r *http.Request, guildID string) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	attrs := map[string]any{"enabled": false, "exempt_roles": []any{}, "exempt_channels": []any{}}
	for k, v := range decodeObject(r) {
		attrs[k] = v
	}
	id := m.id()
	attrs["id"] = id
	attrs["guild_id"] = guildID
	m.automod[id] = attrs
	writeJSON(w, http.StatusOK, attrs)
}

func (m *mockDiscord) serveAutomodItem(w http.ResponseWriter, r *http.Request, _, id string) {
	a, ok := m.automod[id]
	if !ok {
		http.Error(w, `{"message":"Unknown Rule","code":10000}`, http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, a)
	case http.MethodPatch:
		for k, v := range decodeObject(r) {
			a[k] = v
		}
		writeJSON(w, http.StatusOK, a)
	case http.MethodDelete:
		delete(m.automod, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// serveChannelPermission stores/removes a permission overwrite on a channel, so a
// later GET /channels/{id} exposes it under permission_overwrites.
func (m *mockDiscord) serveChannelPermission(w http.ResponseWriter, r *http.Request, channelID, overwriteID string) {
	ch, ok := m.channels[channelID]
	if !ok {
		http.Error(w, `{"message":"Unknown Channel","code":10003}`, http.StatusNotFound)
		return
	}
	overwrites, _ := ch["permission_overwrites"].([]any)
	switch r.Method {
	case http.MethodPut:
		body := decodeObject(r)
		body["id"] = overwriteID
		next := make([]any, 0, len(overwrites)+1)
		for _, o := range overwrites {
			if om, _ := o.(map[string]any); om["id"] != overwriteID {
				next = append(next, o)
			}
		}
		next = append(next, body)
		ch["permission_overwrites"] = next
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		next := make([]any, 0, len(overwrites))
		for _, o := range overwrites {
			if om, _ := o.(map[string]any); om["id"] != overwriteID {
				next = append(next, o)
			}
		}
		ch["permission_overwrites"] = next
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (m *mockDiscord) serveRolesCollection(w http.ResponseWriter, r *http.Request, _ string) {
	switch r.Method {
	case http.MethodPost:
		attrs := map[string]any{
			"color": float64(0), "hoist": false, "mentionable": false,
			"permissions": "0", "managed": false, "position": float64(1),
		}
		for k, v := range decodeObject(r) {
			attrs[k] = v
		}
		id := m.id()
		attrs["id"] = id
		m.roles[id] = attrs
		writeJSON(w, http.StatusOK, attrs)
	case http.MethodGet:
		out := make([]map[string]any, 0, len(m.roles))
		for _, a := range m.roles {
			out = append(out, a)
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPatch:
		// modify-role-positions: a [{id, position}] array. Apply and echo the list.
		for _, entry := range decodeArray(r) {
			e, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if id, _ := e["id"].(string); id != "" {
				if a, ok := m.roles[id]; ok {
					a["position"] = e["position"]
				}
			}
		}
		out := make([]map[string]any, 0, len(m.roles))
		for _, a := range m.roles {
			out = append(out, a)
		}
		writeJSON(w, http.StatusOK, out)
	default:
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (m *mockDiscord) serveRoleItem(w http.ResponseWriter, r *http.Request, id string) {
	a, ok := m.roles[id]
	if !ok {
		http.Error(w, `{"message":"Unknown Role","code":10011}`, http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		for k, v := range decodeObject(r) {
			a[k] = v
		}
		writeJSON(w, http.StatusOK, a)
	case http.MethodDelete:
		delete(m.roles, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (m *mockDiscord) serveChannelsCollection(w http.ResponseWriter, r *http.Request, guildID string) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	attrs := map[string]any{
		"nsfw": false, "position": float64(0), "rate_limit_per_user": float64(0),
		"bitrate": float64(0), "user_limit": float64(0),
		"default_thread_rate_limit_per_user": float64(0),
	}
	for k, v := range decodeObject(r) {
		attrs[k] = v
	}
	id := m.id()
	attrs["id"] = id
	attrs["guild_id"] = guildID
	m.channels[id] = attrs
	writeJSON(w, http.StatusOK, attrs)
}

func (m *mockDiscord) serveChannelItem(w http.ResponseWriter, r *http.Request, id string) {
	a, ok := m.channels[id]
	if !ok {
		http.Error(w, `{"message":"Unknown Channel","code":10003}`, http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, a)
	case http.MethodPatch:
		for k, v := range decodeObject(r) {
			a[k] = v
		}
		writeJSON(w, http.StatusOK, a)
	case http.MethodDelete:
		delete(m.channels, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"message":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// importIDFunc builds an ImportState id by joining the named state attributes
// with "/", matching each resource's ImportState format.
func importIDFunc(rn string, attrNames ...string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[rn]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", rn)
		}
		parts := make([]string, len(attrNames))
		for i, name := range attrNames {
			parts[i] = rs.Primary.Attributes[name]
		}
		return strings.Join(parts, "/"), nil
	}
}

func decodeObject(r *http.Request) map[string]any {
	defer func() { _ = r.Body.Close() }()
	data, _ := io.ReadAll(r.Body)
	var body map[string]any
	if len(data) == 0 || json.Unmarshal(data, &body) != nil {
		return map[string]any{}
	}
	return body
}

func decodeArray(r *http.Request) []any {
	defer func() { _ = r.Body.Close() }()
	data, _ := io.ReadAll(r.Body)
	var body []any
	if len(data) == 0 || json.Unmarshal(data, &body) != nil {
		return nil
	}
	return body
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
