package paymentexpiry_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	runtimeconfig "github.com/code-corhuila/multi-tour-worker-runtime/config"
	"github.com/code-corhuila/multi-tour-worker-runtime/httpclient"
	"github.com/code-corhuila/multi-tour-worker-runtime/runner"

	reservationshttp "github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/adapter/out/http"
	"github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/application/usecase"
)

const (
	tenant = "11111111-1111-4111-8111-111111111111"
	first  = "33333333-3333-4333-8333-333333333333"
	second = "44444444-4444-4444-8444-444444444444"
	token  = "service-token-value"
	base   = "/api/v1/commercial-reservation/internal/tenants/" + tenant
)

// HU-WRK-001 AC1: one run of the whole worker (runner, use case and client) against a service that lists two due reservations and
// expires one of them: both calls of the run carry the token and the same correlation id, and the second run lists nothing.
func TestHU_WRK_001_AC1_one_run_expires_the_due_reservations_with_one_correlation_id(t *testing.T) {
	var mu sync.Mutex
	var paths, correlations []string
	due := []string{first, second}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		correlations = append(correlations, r.Header.Get("X-Correlation-Id"))
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == base+"/payment-deadlines":
			var items []string
			for _, id := range due {
				items = append(items, `{"reservationId":"`+id+`","kind":"INITIAL","frozen":false}`)
			}
			_, _ = w.Write([]byte(`{"data":[` + strings.Join(items, ",") + `],"meta":{"page":1,"limit":100,"total":2,"totalPages":1}}`))
		case r.Method == http.MethodPost && r.URL.Path == base+"/reservations/"+first+"/expiry":
			due = []string{second}
			_, _ = w.Write([]byte(`{"reservationId":"` + first + `","expired":true,"cause":"PAYMENT_EXPIRY","reservationStatus":"CANCELLED"}`))
		case r.Method == http.MethodPost && r.URL.Path == base+"/reservations/"+second+"/expiry":
			_, _ = w.Write([]byte(`{"reservationId":"` + second + `","expired":false,"reason":"PROOF_IN_VALIDATION","reservationStatus":"PENDING_PAYMENT"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := httpclient.New(api.URL, runtimeconfig.Secret(token))
	job := usecase.NewExpireUnpaid(reservationshttp.NewReservations(client), []string{tenant}, log)
	r, err := runner.New(job, runner.Options{Interval: time.Hour, BatchSize: 100, RunTimeout: 10 * time.Second, Logger: log})
	if err != nil {
		t.Fatal(err)
	}

	res := r.RunOnce(t.Context())

	if res.Processed != 2 || res.Failed != 0 {
		t.Fatalf("processed %d failed %d, want 2 and 0", res.Processed, res.Failed)
	}
	if len(paths) != 3 || paths[0] != "GET "+base+"/payment-deadlines" {
		t.Fatalf("paths = %v", paths)
	}
	for _, c := range correlations {
		if c == "" || c != res.CorrelationID {
			t.Errorf("correlation %q, want the run's %q on every request", c, res.CorrelationID)
		}
	}
}

// Annex D: when the service fails, the run is counted as failed and the next run works.
func TestHU_WRK_001_AC1_a_failing_service_fails_the_run_and_the_next_one_recovers(t *testing.T) {
	var mu sync.Mutex
	down := true
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if down {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"INTERNAL_ERROR","message":"x","traceId":"t-1"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[],"meta":{"page":1,"limit":100,"total":0,"totalPages":0}}`))
	}))
	defer api.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := httpclient.New(api.URL, runtimeconfig.Secret(token), httpclient.WithAttempts(1))
	job := usecase.NewExpireUnpaid(reservationshttp.NewReservations(client), []string{tenant}, log)
	r, _ := runner.New(job, runner.Options{Interval: time.Hour, BatchSize: 100, RunTimeout: 10 * time.Second, Logger: log})

	bad := r.RunOnce(t.Context())
	mu.Lock()
	down = false
	mu.Unlock()
	good := r.RunOnce(t.Context())

	if bad.Failed != 1 || good.Failed != 0 {
		t.Errorf("failed %d then %d, want 1 then 0", bad.Failed, good.Failed)
	}
}
