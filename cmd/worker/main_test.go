package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The worker exposes only /health and it answers that the process is up (Annex D, Annex I).
func TestI_health_answers(t *testing.T) {
	srv := httptest.NewServer(newMux())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if !strings.Contains(string(body), `"ok"`) {
		t.Errorf("body = %q, want it to say ok", body)
	}
}

func TestI_health_is_the_only_route(t *testing.T) {
	srv := httptest.NewServer(newMux())
	defer srv.Close()

	for _, path := range []string{"/", "/metrics", "/internal/payment-deadlines"} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, res.StatusCode)
		}
	}
}
