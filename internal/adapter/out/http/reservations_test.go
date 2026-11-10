package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/code-corhuila/multi-tour-worker-runtime/config"
	"github.com/code-corhuila/multi-tour-worker-runtime/correlation"
	"github.com/code-corhuila/multi-tour-worker-runtime/httpclient"

	reservationshttp "github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/adapter/out/http"
)

const (
	tenant      = "11111111-1111-4111-8111-111111111111"
	reservation = "33333333-3333-4333-8333-333333333333"
	token       = "service-token-value"
	base        = "/api/v1/commercial-reservation/internal/tenants/" + tenant
)

type seen struct {
	method, path, query, auth, correlation, key string
}

// stub is commercial-reservation: it records each request and answers with the next scripted reply.
type stub struct {
	mu      sync.Mutex
	seen    []seen
	replies []func(w http.ResponseWriter)
}

func (s *stub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.seen = append(s.seen, seen{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"),
		r.Header.Get(correlation.Header), r.Header.Get("Idempotency-Key")})
	reply := s.replies[min(len(s.seen), len(s.replies))-1]
	s.mu.Unlock()
	reply(w)
}

func write(status int, body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func adapter(t *testing.T, replies ...func(http.ResponseWriter)) (*reservationshttp.Reservations, *stub) {
	t.Helper()
	s := &stub{replies: replies}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	client := httpclient.New(srv.URL, config.Secret(token), httpclient.WithAttempts(3),
		httpclient.WithSleeper(func(context.Context, time.Duration) error { return nil }))
	return reservationshttp.NewReservations(client), s
}

func run() context.Context { return correlation.WithID(context.Background(), "run-0000-aaaa") }

const dueList = `{"data":[{"reservationId":"` + reservation + `","kind":"INITIAL","dueAt":"2026-11-10T10:00:00Z","effectiveDeadline":"2026-11-10T10:00:00Z","frozen":false}],"meta":{"page":1,"limit":100,"total":1,"totalPages":1}}`

// HU-WRK-001 AC1: it reads the {data, meta} list of due deadlines of the tenant of the path, with the token and the run's correlation id.
func TestListDue_reads_the_due_list_with_the_token_and_the_correlation_id(t *testing.T) {
	a, s := adapter(t, write(200, dueList))

	got, err := a.ListDue(run(), tenant, 50)

	if err != nil || len(got) != 1 || got[0].ReservationID != reservation {
		t.Fatalf("got %+v, %v", got, err)
	}
	r := s.seen[0]
	if r.method != "GET" || r.path != base+"/payment-deadlines" || r.query != "due=true&page=1&limit=50" {
		t.Errorf("request = %+v", r)
	}
	if r.auth != "Bearer "+token || r.correlation != "run-0000-aaaa" {
		t.Errorf("credentials = %q, correlation %q", r.auth, r.correlation)
	}
}

// HU-WRK-001 AC1: the expiry is a POST with an Idempotency-Key and the token, and its answer says whether the service expired it.
func TestExpire_posts_with_the_key_and_reads_what_the_service_decided(t *testing.T) {
	a, s := adapter(t, write(200, `{"reservationId":"`+reservation+`","expired":true,"cause":"PAYMENT_EXPIRY","reservationStatus":"CANCELLED"}`),
		write(200, `{"reservationId":"`+reservation+`","expired":false,"reason":"PROOF_IN_VALIDATION","reservationStatus":"PENDING_PAYMENT"}`))

	first, err1 := a.Expire(run(), tenant, reservation, "expiry-key-0001")
	second, err2 := a.Expire(run(), tenant, reservation, "expiry-key-0002")

	if err1 != nil || err2 != nil || !first.Expired || second.Expired || second.Reason != "PROOF_IN_VALIDATION" {
		t.Fatalf("got %+v %v / %+v %v", first, err1, second, err2)
	}
	r := s.seen[0]
	if r.method != "POST" || r.path != base+"/reservations/"+reservation+"/expiry" || r.key != "expiry-key-0001" {
		t.Errorf("request = %+v", r)
	}
	if r.auth != "Bearer "+token || r.correlation != "run-0000-aaaa" {
		t.Errorf("credentials = %q, correlation %q", r.auth, r.correlation)
	}
}

// Annex D: 5xx and 429 are retried up to the limit; the repeated POST carries the same key.
func TestExpire_retries_a_503_with_the_same_key_and_stops_at_the_limit(t *testing.T) {
	a, s := adapter(t, write(503, `{"error":"UNAVAILABLE","message":"x","traceId":"t1"}`),
		write(429, `{"error":"TOO_MANY","message":"x","traceId":"t2"}`),
		write(200, `{"reservationId":"`+reservation+`","expired":true,"reservationStatus":"CANCELLED"}`))

	got, err := a.Expire(run(), tenant, reservation, "expiry-key-0001")

	if err != nil || !got.Expired || len(s.seen) != 3 || s.seen[0].key != s.seen[2].key {
		t.Fatalf("got %+v %v after %d calls", got, err, len(s.seen))
	}
	a, s = adapter(t, write(503, `{"error":"UNAVAILABLE","message":"x","traceId":"t1"}`))
	if _, err := a.Expire(run(), tenant, reservation, "expiry-key-0001"); err == nil || len(s.seen) != 3 {
		t.Errorf("error %v after %d calls, want a failure after the 3 attempts", err, len(s.seen))
	}
}

// Annex D: any other 4xx is not retried.
func TestExpire_does_not_retry_a_4xx(t *testing.T) {
	for _, status := range []int{401, 403, 404, 422} {
		a, s := adapter(t, write(status, `{"error":"X","message":"no","traceId":"t"}`))

		if _, err := a.Expire(run(), tenant, reservation, "expiry-key-0001"); err == nil || len(s.seen) != 1 {
			t.Errorf("%d: error %v after %d calls, want one call and a failure", status, err, len(s.seen))
		}
	}
}

// An id that is not a UUID never reaches a URL path.
func TestAdapter_refuses_an_id_that_is_not_a_uuid_before_calling(t *testing.T) {
	a, s := adapter(t, write(200, dueList))

	_, err1 := a.Expire(run(), tenant, "../../x", "expiry-key-0001")
	_, err2 := a.ListDue(run(), "acme", 10)
	_, err3 := a.ListDue(run(), tenant, 0)

	if err1 == nil || err2 == nil || err3 == nil || len(s.seen) != 0 {
		t.Errorf("errors %v %v %v, calls %d, want three refusals and no call", err1, err2, err3, len(s.seen))
	}
}

// A malformed 2xx is a failure of the item, and no body text is copied into the error.
func TestListDue_a_body_that_is_not_the_contract_is_an_error(t *testing.T) {
	a, _ := adapter(t, write(200, `{"data":[{"reservationId":"nope"}],"meta":{}}`))

	_, err := a.ListDue(run(), tenant, 10)

	if err == nil || strings.Contains(err.Error(), "nope") {
		t.Errorf("error = %v, want a refusal that does not echo the body", err)
	}
}
