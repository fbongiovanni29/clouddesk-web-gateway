package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

// serviceStatus holds the result of probing a single upstream.
type serviceStatus struct {
	Status string `json:"status"` // "ok" or "unavailable"
}

// healthzResponse is the full /healthz payload.
type healthzResponse struct {
	Status   string                   `json:"status"`
	Services map[string]serviceStatus `json:"services"`
}

func main() {
	authURL := envOrDefault("AUTH_SERVICE_URL", "http://clouddesk-auth-service.default.svc:8080")
	docsURL := envOrDefault("DOCS_API_URL", "http://clouddesk-docs-api.default.svc:8080")
	notifURL := envOrDefault("NOTIFICATIONS_SERVICE_URL", "http://clouddesk-notifications-worker.default.svc:8080")

	type route struct {
		prefix string
		target string
	}

	routes := []route{
		{prefix: "/auth/", target: authURL},
		{prefix: "/docs/", target: docsURL},
		{prefix: "/notifications/", target: notifURL},
	}

	mux := http.NewServeMux()

	for _, r := range routes {
		proxy := newProxy(r.target, r.prefix)
		mux.Handle(r.prefix, proxy)
		log.Printf("route %s -> %s", r.prefix, r.target)
	}

	// /healthz — probe each upstream and report status
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		services := map[string]string{
			"auth":          authURL,
			"docs":          docsURL,
			"notifications": notifURL,
		}

		statuses := make(map[string]serviceStatus, len(services))
		overallOK := true

		client := &http.Client{Timeout: 3 * time.Second}
		for name, base := range services {
			probe := strings.TrimRight(base, "/") + "/healthz"
			resp, err := client.Get(probe)
			if err != nil || resp.StatusCode >= 500 {
				statuses[name] = serviceStatus{Status: "unavailable"}
				overallOK = false
			} else {
				statuses[name] = serviceStatus{Status: "ok"}
			}
			if resp != nil {
				resp.Body.Close()
			}
		}

		overall := "ok"
		if !overallOK {
			overall = "degraded"
		}

		payload := healthzResponse{
			Status:   overall,
			Services: statuses,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(payload)
	})

	// / — service discovery / status
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"service":"web-gateway","status":"ok","routes":["/auth/","/docs/","/notifications/","/healthz"]}`)
	})

	log.Println("web-gateway listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func newProxy(target, prefix string) http.Handler {
	targetURL, err := url.Parse(target)
	if err != nil {
		log.Fatalf("invalid target url %s: %v", target, err)
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.URL.Path = stripPrefix(req.URL.Path, prefix)
			req.Host = targetURL.Host
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("proxy error for %s: %v", r.URL.Path, err)
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"upstream unavailable"}`, http.StatusBadGateway)
		},
	}
	return proxy
}

func stripPrefix(path, prefix string) string {
	stripped := strings.TrimPrefix(path, strings.TrimSuffix(prefix, "/"))
	if stripped == "" || stripped[0] != '/' {
		stripped = "/" + stripped
	}
	return stripped
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
