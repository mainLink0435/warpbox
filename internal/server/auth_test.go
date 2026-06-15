package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ben/warpbox/internal/metadata"
	"github.com/ben/warpbox/internal/throttle"
)

// testHandler returns a simple 200 OK handler for testing middleware.
func testHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func TestRequireAuthDisabled(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	defer store.Close()

	srv := New(Config{Version: "test",  AuthEnabled: false}, store, nil, nil)

	handler := srv.requireAuth(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK when auth disabled, got %d", resp.StatusCode)
	}
}

func TestRequireAuthNoCredentials(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	defer store.Close()

	srv := New(Config{Version: "test",  AuthEnabled: true, AuthUsername: "admin", AuthPassword: "secret"}, store, nil, nil)

	handler := srv.requireAuth(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}

	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header to be set")
	}
}

func TestRequireAuthWrongPassword(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	defer store.Close()

	srv := New(Config{Version: "test",  AuthEnabled: true, AuthUsername: "admin", AuthPassword: "secret"}, store, nil, nil)

	handler := srv.requireAuth(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "wrong")
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}
}

func TestRequireAuthValidCredentials(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	defer store.Close()

	srv := New(Config{Version: "test",  AuthEnabled: true, AuthUsername: "admin", AuthPassword: "secret"}, store, nil, nil)

	handler := srv.requireAuth(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}
}

func TestRequireAuthHealthzExempt(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	defer store.Close()

	srv := New(Config{Version: "test",  AuthEnabled: true, AuthUsername: "admin", AuthPassword: "secret"}, store, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.handleHealthz(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for healthz without auth, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", body["status"])
	}
}

func TestRequireAuthLandingProtected(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	defer store.Close()

	queue := throttle.NewQueue(250)
	srv := New(Config{Version: "test",  AuthEnabled: true, AuthUsername: "admin", AuthPassword: "secret"}, store, nil, queue)

	handler := srv.requireAuth(srv.handleLanding)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for landing page without auth, got %d", resp.StatusCode)
	}
}

func TestRequireAuthLandingWithCredentials(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	defer store.Close()

	queue := throttle.NewQueue(250)
	srv := New(Config{Version: "test",  AuthEnabled: true, AuthUsername: "admin", AuthPassword: "secret"}, store, nil, queue)

	handler := srv.requireAuth(srv.handleLanding)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for landing page with auth, got %d", resp.StatusCode)
	}
}
