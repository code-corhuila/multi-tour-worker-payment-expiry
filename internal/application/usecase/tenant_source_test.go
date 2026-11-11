package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/application/port/out"
)

type tenantSource struct {
	ids   []string
	err   error
	calls int
}

func (s *tenantSource) ListActive(context.Context) ([]string, error) {
	s.calls++
	return s.ids, s.err
}

// HU-WRK-001 DP-08: with no tenant list configured the job sweeps the tenants the service names, asked at each run.
func TestTenantSource_is_asked_when_the_configured_list_is_empty(t *testing.T) {
	f := &fake{due: map[string][]out.Deadline{tenantA: deadlines("r1"), tenantB: deadlines("r2", "r3")}}
	src := &tenantSource{ids: []string{tenantA}}
	j := job(f)
	j.WithTenantSource(src)

	processed, failed := j.Run(context.Background(), 100)
	j.Run(context.Background(), 100)

	if processed != 1 || failed != 0 || src.calls != 2 {
		t.Errorf("processed %d failed %d asked %d, want 1, 0 and 2 (once per run)", processed, failed, src.calls)
	}
}

// TENANT_IDS stays the fallback: a configured list wins and the service is not asked.
func TestTenantSource_is_not_asked_when_the_list_is_configured(t *testing.T) {
	f := &fake{due: map[string][]out.Deadline{tenantA: deadlines("r1"), tenantB: deadlines("r2", "r3")}}
	src := &tenantSource{ids: []string{tenantA}}
	j := job(f, tenantB)
	j.WithTenantSource(src)

	processed, _ := j.Run(context.Background(), 100)

	if processed != 2 || src.calls != 0 {
		t.Errorf("processed %d asked %d, want the 2 of tenant B and no question", processed, src.calls)
	}
}

// A list that cannot be read counts as a failed run and sweeps nothing; the next run asks again.
func TestTenantSource_a_failure_sweeps_nothing_and_counts_once(t *testing.T) {
	f := &fake{due: map[string][]out.Deadline{tenantA: deadlines("r1"), tenantB: deadlines("r2", "r3")}}
	j := job(f)
	j.WithTenantSource(&tenantSource{err: errors.New("service down")})

	processed, failed := j.Run(context.Background(), 100)

	if processed != 0 || failed != 1 {
		t.Errorf("processed %d failed %d, want 0 and 1", processed, failed)
	}
}
