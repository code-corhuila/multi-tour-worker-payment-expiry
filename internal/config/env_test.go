package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/config"
)

const (
	tenantA = "11111111-1111-4111-8111-111111111111"
	tenantB = "22222222-2222-4222-8222-222222222222"
)

func env(pairs ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return func(k string) string { return m[k] }
}

var required = []string{
	"SERVICE_TOKEN", "secret-token-value",
	"COMMERCIAL_RESERVATION_BASE_URL", "http://commercial-reservation-api:8083/",
	"TENANT_IDS", tenantA,
}

func with(extra ...string) []string { return append(append([]string{}, required...), extra...) }

// HU-WRK-001 AC1: every minute is the default interval (ADR-004 D10), and the shared runtime variables travel with it.
func TestLoadExpiry_applies_the_declared_defaults(t *testing.T) {
	got, err := config.LoadExpiry(env(required...))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Interval != time.Minute || got.HealthAddr != ":8080" || got.Runtime.BatchSize != 100 {
		t.Errorf("defaults = %+v", got)
	}
	if got.CommercialReservationURL != "http://commercial-reservation-api:8083" {
		t.Errorf("url = %q, the trailing slash should go", got.CommercialReservationURL)
	}
}

func TestLoadExpiry_reads_its_own_variables_and_the_tenant_list_without_repeats(t *testing.T) {
	got, err := config.LoadExpiry(env(with("EXPIRY_INTERVAL", "30s", "HEALTH_ADDR", ":9090",
		"TENANT_IDS", tenantA+" , "+tenantB+","+tenantA)...))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Interval != 30*time.Second || got.HealthAddr != ":9090" {
		t.Errorf("got %+v", got)
	}
	if len(got.TenantIDs) != 2 || got.TenantIDs[0] != tenantA || got.TenantIDs[1] != tenantB {
		t.Errorf("tenants = %v, want the two ids once each in order", got.TenantIDs)
	}
}

// A missing or malformed variable stops the worker with its name, never its value.
func TestLoadExpiry_names_the_bad_variable_and_never_prints_a_value(t *testing.T) {
	cases := map[string][]string{
		"SERVICE_TOKEN":                   {"COMMERCIAL_RESERVATION_BASE_URL", "http://x:8083", "TENANT_IDS", tenantA},
		"COMMERCIAL_RESERVATION_BASE_URL": {"SERVICE_TOKEN", "secret-token-value", "TENANT_IDS", tenantA},
		"EXPIRY_INTERVAL":                 with("EXPIRY_INTERVAL", "soon"),
	}
	for name, pairs := range cases {
		_, err := config.LoadExpiry(env(pairs...))
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: error = %v, want one that names the variable", name, err)
			continue
		}
		if strings.Contains(err.Error(), "secret-token-value") {
			t.Errorf("%s: the error prints a secret: %v", name, err)
		}
	}
}

// The tenant id goes into a URL path, so anything that is not a UUID is refused at start.
func TestLoadExpiry_refuses_a_tenant_that_is_not_a_uuid(t *testing.T) {
	for _, bad := range []string{"acme", tenantA + "/../x", tenantA + ",", "../" + tenantA} {
		if _, err := config.LoadExpiry(env(with("TENANT_IDS", bad)...)); err == nil || !strings.Contains(err.Error(), "TENANT_IDS") {
			t.Errorf("TENANT_IDS=%q: error = %v, want a refusal that names it", bad, err)
		}
	}
}

func TestLoadExpiry_refuses_a_base_url_with_a_path_or_credentials(t *testing.T) {
	for _, bad := range []string{"http://user:pw@host:8083", "http://host:8083/api", "ftp://host", "http://host:8083?x=1"} {
		if _, err := config.LoadExpiry(env(with("COMMERCIAL_RESERVATION_BASE_URL", bad)...)); err == nil {
			t.Errorf("url %q was accepted", bad)
		}
	}
}

// HU-WRK-001 DP-08: TENANT_IDS is optional. Empty means the tenants come from the service at each run; a malformed one still stops the worker.
func TestTenantIDs_empty_leaves_the_list_to_the_service(t *testing.T) {
	got, err := config.LoadExpiry(env("SERVICE_TOKEN", "secret-token-value", "COMMERCIAL_RESERVATION_BASE_URL", "http://x:8083", "TENANT_IDS", " "))

	if err != nil || len(got.TenantIDs) != 0 {
		t.Fatalf("got %v, %v", got.TenantIDs, err)
	}
}

func TestTenantIDs_malformed_still_stops_the_worker_naming_the_variable(t *testing.T) {
	_, err := config.LoadExpiry(env(with("TENANT_IDS", "not-a-uuid")...))

	if err == nil || !strings.Contains(err.Error(), "TENANT_IDS") {
		t.Errorf("error = %v", err)
	}
}
