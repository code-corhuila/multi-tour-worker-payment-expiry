package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/code-corhuila/multi-tour-worker-runtime/config"
	"github.com/code-corhuila/multi-tour-worker-runtime/httpclient"

	tenantshttp "github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/adapter/out/http"
)

const (
	tenantOne = "11111111-1111-4111-8111-111111111111"
	tenantTwo = "22222222-2222-4222-8222-222222222222"
)

func tenantsAdapter(t *testing.T, replies ...func(http.ResponseWriter)) (*tenantshttp.Tenants, *stub) {
	t.Helper()
	s := &stub{replies: replies}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	client := httpclient.New(srv.URL, config.Secret(token), httpclient.WithAttempts(2),
		httpclient.WithSleeper(func(context.Context, time.Duration) error { return nil }))
	return tenantshttp.NewTenants(client), s
}

// HU-WRK-001 DP-08: the active tenants come from the internal route of commercial-reservation, page by page, with the token of the service.
func TestTenants_reads_every_page_of_the_active_tenants(t *testing.T) {
	a, s := tenantsAdapter(t,
		write(200, `{"data":[{"tenantId":"`+tenantOne+`"}],"meta":{"page":1,"limit":100,"total":2,"totalPages":2}}`),
		write(200, `{"data":[{"tenantId":"`+tenantTwo+`"}],"meta":{"page":2,"limit":100,"total":2,"totalPages":2}}`))

	got, err := a.ListActive(context.Background())

	if err != nil || len(got) != 2 || got[0] != tenantOne || got[1] != tenantTwo {
		t.Fatalf("got %v, %v", got, err)
	}
	if len(s.seen) != 2 || s.seen[0].method != http.MethodGet || s.seen[0].path != "/api/v1/commercial-reservation/internal/tenants" ||
		s.seen[0].query != "status=ACTIVE&page=1&limit=100" || s.seen[1].query != "status=ACTIVE&page=2&limit=100" {
		t.Errorf("requests = %+v", s.seen)
	}
	if s.seen[0].auth != "Bearer "+token {
		t.Errorf("auth = %q, want the service token", s.seen[0].auth)
	}
}

func TestTenants_an_empty_list_is_not_an_error(t *testing.T) {
	a, _ := tenantsAdapter(t, write(200, `{"data":[],"meta":{"page":1,"limit":100,"total":0,"totalPages":0}}`))

	got, err := a.ListActive(context.Background())

	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestTenants_a_refusal_or_a_bad_id_is_an_error_and_never_a_partial_list(t *testing.T) {
	refused, _ := tenantsAdapter(t, write(403, `{"error":{"code":"FORBIDDEN","message":"no"}}`))
	if got, err := refused.ListActive(context.Background()); err == nil || got != nil {
		t.Errorf("a 403 gave %v, %v", got, err)
	}
	bad, _ := tenantsAdapter(t, write(200, `{"data":[{"tenantId":"../x"}],"meta":{"page":1,"limit":100,"total":1,"totalPages":1}}`))
	if got, err := bad.ListActive(context.Background()); err == nil || got != nil || !strings.Contains(err.Error(), "uuid") {
		t.Errorf("a bad id gave %v, %v", got, err)
	}
}
