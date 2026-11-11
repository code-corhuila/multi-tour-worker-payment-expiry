package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	runtimeconfig "github.com/code-corhuila/multi-tour-worker-runtime/config"
)

const defaultInterval = time.Minute // ADR-004 D10, HU-WRK-001 AC1

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Expiry is everything the worker reads from the environment: the shared runtime variables plus the interval, the health
// address, the internal base URL of the only service it calls and the tenants it sweeps.
type Expiry struct {
	Runtime                  runtimeconfig.Config
	Interval                 time.Duration
	HealthAddr               string
	CommercialReservationURL string
	TenantIDs                []string
}

// LoadExpiry reads the variables through getenv. Anything missing or malformed stops the worker at start; an error names
// the variable and never prints a value.
func LoadExpiry(getenv func(string) string) (Expiry, error) {
	shared, err := runtimeconfig.Load(getenv)
	if err != nil {
		return Expiry{}, err
	}
	d := Expiry{Runtime: shared, Interval: defaultInterval, HealthAddr: ":8080"}
	if s := getenv("EXPIRY_INTERVAL"); s != "" {
		if d.Interval, err = time.ParseDuration(s); err != nil || d.Interval <= 0 {
			return Expiry{}, fmt.Errorf("EXPIRY_INTERVAL must be a positive duration")
		}
	}
	if s := getenv("HEALTH_ADDR"); s != "" {
		d.HealthAddr = s
	}
	if d.CommercialReservationURL, err = baseURL(getenv, "COMMERCIAL_RESERVATION_BASE_URL"); err != nil {
		return Expiry{}, err
	}
	if d.TenantIDs, err = tenantIDs(getenv("TENANT_IDS")); err != nil {
		return Expiry{}, err
	}
	return d, nil
}

// tenantIDs reads the comma-separated list. It is optional: empty means the job asks commercial-reservation for the active tenants at each run
// (HU-WRK-001 DP-08) and TENANT_IDS is the fallback. The ids go into URL paths, so each one must be a UUID.
func tenantIDs(raw string) ([]string, error) {
	var ids []string
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		id := strings.ToLower(strings.TrimSpace(part))
		if !uuid.MatchString(id) {
			return nil, fmt.Errorf("TENANT_IDS must be a comma-separated list of tenant UUIDs")
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func baseURL(getenv func(string) string, name string) (string, error) {
	raw := strings.TrimRight(getenv(name), "/")
	u, err := url.Parse(raw)
	// Only scheme://host[:port]: credentials, a query, a fragment or a path would reach every request.
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return "", fmt.Errorf("%s must be an http(s) url with only scheme, host and port", name)
	}
	return raw, nil
}
