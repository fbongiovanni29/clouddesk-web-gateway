package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// startBackend spins up an httptest server that echoes the request path as JSON.
func startBackend(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"upstream_path": r.URL.Path})
	}))
}

// buildMux constructs the gateway mux pointing at the provided upstream URLs.
func buildMux(authURL, docsURL, notifURL string) http.Handler {
	mux := http.NewServeMux()

	routes := []struct {
		prefix string
		target string
	}{
		{"/auth/", authURL},
		{"/docs/", docsURL},
		{"/notifications/", notifURL},
	}

	for _, r := range routes {
		proxy := newProxy(r.target, r.prefix)
		mux.Handle(r.prefix, proxy)
	}

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","services":{"auth":{"status":"ok"},"docs":{"status":"ok"},"notifications":{"status":"ok"}}}`))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"service":"web-gateway","status":"ok","routes":["/auth/","/docs/","/notifications/","/healthz"]}`))
	})

	return mux
}

func TestRootStatus(t *testing.T) {
	auth := startBackend(t)
	defer auth.Close()
	docs := startBackend(t)
	defer docs.Close()
	notif := startBackend(t)
	defer notif.Close()

	gw := httptest.NewServer(buildMux(auth.URL, docs.URL, notif.URL))
	defer gw.Close()

	resp, err := http.Get(gw.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
	if body["service"] != "web-gateway" {
		t.Errorf("expected service web-gateway, got %v", body["service"])
	}
}

func TestAuthProxy(t *testing.T) {
	auth := startBackend(t)
	defer auth.Close()
	docs := startBackend(t)
	defer docs.Close()
	notif := startBackend(t)
	defer notif.Close()

	gw := httptest.NewServer(buildMux(auth.URL, docs.URL, notif.URL))
	defer gw.Close()

	resp, err := http.Get(gw.URL + "/auth/login")
	if err != nil {
		t.Fatalf("GET /auth/login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// After stripping /auth the upstream should see /login
	if !strings.HasPrefix(body["upstream_path"], "/login") {
		t.Errorf("expected upstream path to start with /login, got %q", body["upstream_path"])
	}
}

func TestDocsProxy(t *testing.T) {
	auth := startBackend(t)
	defer auth.Close()
	docs := startBackend(t)
	defer docs.Close()
	notif := startBackend(t)
	defer notif.Close()

	gw := httptest.NewServer(buildMux(auth.URL, docs.URL, notif.URL))
	defer gw.Close()

	resp, err := http.Get(gw.URL + "/docs/api/v1")
	if err != nil {
		t.Fatalf("GET /docs/api/v1: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(body["upstream_path"], "/api/v1") {
		t.Errorf("expected upstream path /api/v1, got %q", body["upstream_path"])
	}
}

func TestNotificationsProxy(t *testing.T) {
	auth := startBackend(t)
	defer auth.Close()
	docs := startBackend(t)
	defer docs.Close()
	notif := startBackend(t)
	defer notif.Close()

	gw := httptest.NewServer(buildMux(auth.URL, docs.URL, notif.URL))
	defer gw.Close()

	resp, err := http.Get(gw.URL + "/notifications/subscribe")
	if err != nil {
		t.Fatalf("GET /notifications/subscribe: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(body["upstream_path"], "/subscribe") {
		t.Errorf("expected upstream path /subscribe, got %q", body["upstream_path"])
	}
}

func TestHealthzEndpoint(t *testing.T) {
	auth := startBackend(t)
	defer auth.Close()
	docs := startBackend(t)
	defer docs.Close()
	notif := startBackend(t)
	defer notif.Close()

	gw := httptest.NewServer(buildMux(auth.URL, docs.URL, notif.URL))
	defer gw.Close()

	resp, err := http.Get(gw.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] == nil {
		t.Error("expected status field in healthz response")
	}
	if body["services"] == nil {
		t.Error("expected services field in healthz response")
	}
}
