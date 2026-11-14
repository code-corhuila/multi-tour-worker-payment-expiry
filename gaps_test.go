package paymentexpiry_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	runtimeconfig "github.com/code-corhuila/multi-tour-worker-runtime/config"
	"github.com/code-corhuila/multi-tour-worker-runtime/httpclient"
	"github.com/code-corhuila/multi-tour-worker-runtime/logging"
	"github.com/code-corhuila/multi-tour-worker-runtime/runner"

	reservationshttp "github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/adapter/out/http"
	"github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/application/usecase"
)

// service is commercial-reservation as the worker meets it over HTTP: the list it serves and the answer it gives to each expiry.
type service struct {
	mu      sync.Mutex
	list    string
	answers map[string]string
	posts   []string
	bodies  []string
}

func (s *service) handler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte(s.list))
		return
	}
	b, _ := io.ReadAll(r.Body)
	s.posts = append(s.posts, r.URL.Path)
	s.bodies = append(s.bodies, string(b))
	for id, answer := range s.answers {
		if strings.Contains(r.URL.Path, id) {
			_, _ = w.Write([]byte(answer))
			return
		}
	}
	_, _ = w.Write([]byte(`{"expired":true}`))
}

func page(items ...string) string {
	return `{"data":[` + strings.Join(items, ",") + `],"meta":{"page":1,"limit":100,"total":` + strconv.Itoa(len(items)) + `,"totalPages":1}}`
}

func runOnce(t *testing.T, s *service, logs io.Writer) runner.Result {
	t.Helper()
	api := httptest.NewServer(http.HandlerFunc(s.handler))
	t.Cleanup(api.Close)
	log := logging.New(logs)
	client := httpclient.New(api.URL, runtimeconfig.Secret(token), httpclient.WithAttempts(1))
	job := usecase.NewExpireUnpaid(reservationshttp.NewReservations(client), []string{tenant}, log)
	r, err := runner.New(job, runner.Options{Interval: time.Hour, BatchSize: 100, RunTimeout: 10 * time.Second, Logger: log})
	if err != nil {
		t.Fatal(err)
	}
	return r.RunOnce(t.Context())
}

// HU-WRK-001 AC2: no deadline passes the start of the departure, and that cap (RN-EJE-001, spec 011 AC-11.6) is the service's. The worker
// holds no date logic: it asks the expiry of every reservation the list names, whatever dates the item carries, and sends no date of its own,
// so it can neither expire early nor extend past the departure.
func TestHU_WRK_001_AC2_the_departure_cap_is_the_services_the_worker_filters_by_no_date(t *testing.T) {
	s := &service{list: page(
		`{"reservationId":"`+first+`","kind":"INITIAL","dueAt":"2099-01-01T00:00:00Z","departureStartsAt":"2020-01-01T00:00:00Z"}`,
		`{"reservationId":"`+second+`","kind":"INITIAL","dueAt":"2020-01-01T00:00:00Z","departureStartsAt":"2099-01-01T00:00:00Z"}`),
		answers: map[string]string{second: `{"expired":false,"reason":"NOT_DUE"}`}}

	res := runOnce(t, s, io.Discard)

	if len(s.posts) != 2 || !strings.Contains(s.posts[0], first) || !strings.Contains(s.posts[1], second) {
		t.Fatalf("expiry calls = %v, want one for each listed reservation whatever its dates", s.posts)
	}
	for _, b := range s.bodies {
		if strings.TrimSpace(b) != "" {
			t.Errorf("the expiry carries a body %q: the worker must send no date, the service decides", b)
		}
	}
	if res.Processed != 2 || res.Failed != 0 {
		t.Errorf("processed %d failed %d, want 2 and 0: NOT_DUE is the service answer, not a failure", res.Processed, res.Failed)
	}
}

// HU-WRK-001 D-1 (title: unpaid reservations and balances; spec 011 AC-11.14): a balance deadline of a confirmed reservation is expired
// like an initial one, the worker does not filter by kind.
func TestHU_WRK_001_D1_a_balance_deadline_is_expired_like_an_initial_one(t *testing.T) {
	s := &service{list: page(
		`{"reservationId":"`+first+`","kind":"INITIAL"}`,
		`{"reservationId":"`+second+`","kind":"BALANCE"}`)}

	res := runOnce(t, s, io.Discard)

	if len(s.posts) != 2 || !strings.Contains(s.posts[1], second) || res.Processed != 2 || res.Failed != 0 {
		t.Fatalf("posts %v processed %d failed %d, want both kinds expired", s.posts, res.Processed, res.Failed)
	}
}

// HU-WRK-001 AC3 (spec 011 AC-11.15, HU-COM-020): with the tenant inactive the service lists nothing, so the worker asks no expiry and counts
// no failure; and a tenant that goes inactive between the list and the expiry is answered TENANT_INACTIVE once, not retried, not a failure.
func TestHU_WRK_001_AC3_an_inactive_tenant_is_left_frozen_without_expiry_or_failure(t *testing.T) {
	frozen := &service{list: page()}
	res := runOnce(t, frozen, io.Discard)
	if len(frozen.posts) != 0 || res.Failed != 0 {
		t.Errorf("inactive tenant: posts %v failed %d, want none", frozen.posts, res.Failed)
	}

	race := &service{list: page(`{"reservationId":"` + first + `","kind":"INITIAL"}`),
		answers: map[string]string{first: `{"expired":false,"reason":"TENANT_INACTIVE"}`}}
	res = runOnce(t, race, io.Discard)
	if len(race.posts) != 1 || res.Failed != 0 || res.Processed != 1 {
		t.Errorf("race: posts %v processed %d failed %d, want one post and no failure", race.posts, res.Processed, res.Failed)
	}
}

// Spec 011 AC-11.15 / Annex D: the log lines carry the tenant and the reservation and never the credential.
func TestHU_WRK_001_D7_log_lines_carry_the_ids_and_never_the_token(t *testing.T) {
	s := &service{list: page(`{"reservationId":"` + first + `","kind":"INITIAL"}`)}
	var logs bytes.Buffer

	runOnce(t, s, &logs)

	out := logs.String()
	if strings.Contains(out, token) || strings.Contains(strings.ToLower(out), "bearer") {
		t.Errorf("the log carries the credential: %s", out)
	}
	for _, want := range []string{tenant, first} {
		if !strings.Contains(out, want) {
			t.Errorf("the log does not carry %q: %s", want, out)
		}
	}
}

// HU-WRK-001 AC1 (every minute): each tick of the runner is one run that lists the due deadlines and asks the expiry.
func TestHU_WRK_001_AC1_each_interval_is_one_run(t *testing.T) {
	var mu sync.Mutex
	posts := 0
	expired := make(chan struct{}, 8)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(page(`{"reservationId":"` + first + `","kind":"INITIAL"}`)))
			return
		}
		posts++
		expired <- struct{}{}
		_, _ = w.Write([]byte(`{"expired":true}`))
	}))
	defer api.Close()
	log := logging.New(io.Discard)
	job := usecase.NewExpireUnpaid(reservationshttp.NewReservations(httpclient.New(api.URL, runtimeconfig.Secret(token))), []string{tenant}, log)
	ticks := make(chan time.Time)
	r, err := runner.New(job, runner.Options{Interval: time.Minute, BatchSize: 100, RunTimeout: 10 * time.Second, Logger: log, Ticks: ticks})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	ctx, stop := context.WithCancel(t.Context())
	go func() { r.Start(ctx); close(done) }()

	for i := 1; i <= 3; i++ {
		ok := false
		for attempt := 0; attempt < 100 && !ok; attempt++ {
			ticks <- time.Now()
			select {
			case <-expired:
				ok = true
			case <-time.After(time.Second):
			}
		}
		if !ok {
			t.Fatalf("interval %d did not run the job", i)
		}
	}
	stop()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if posts != 3 {
		t.Errorf("expiry calls = %d, want one per interval (3)", posts)
	}
}
