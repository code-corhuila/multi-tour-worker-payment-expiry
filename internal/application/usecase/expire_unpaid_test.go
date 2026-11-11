package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	workerruntime "github.com/code-corhuila/multi-tour-worker-runtime"
	"github.com/code-corhuila/multi-tour-worker-runtime/correlation"

	"github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/application/port/out"
	"github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/application/usecase"
)

const (
	tenantA = "11111111-1111-4111-8111-111111111111"
	tenantB = "22222222-2222-4222-8222-222222222222"
)

type expiry struct{ tenant, reservation, key string }

// fake is commercial-reservation as the job sees it.
type fake struct {
	mu       sync.Mutex
	due      map[string][]out.Deadline
	listErr  map[string]error
	expireBy func(reservation string) (out.Outcome, error)
	limits   []int
	calls    []expiry
}

func (f *fake) ListDue(_ context.Context, tenant string, limit int) ([]out.Deadline, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.limits = append(f.limits, limit)
	if err := f.listErr[tenant]; err != nil {
		return nil, err
	}
	return f.due[tenant], nil
}

func (f *fake) Expire(_ context.Context, tenant, reservation, key string) (out.Outcome, error) {
	f.mu.Lock()
	f.calls = append(f.calls, expiry{tenant, reservation, key})
	f.mu.Unlock()
	if f.expireBy != nil {
		return f.expireBy(reservation)
	}
	return out.Outcome{Expired: true}, nil
}

func deadlines(ids ...string) []out.Deadline {
	var d []out.Deadline
	for _, id := range ids {
		d = append(d, out.Deadline{ReservationID: id})
	}
	return d
}

func job(f *fake, tenants ...string) *usecase.ExpireUnpaid {
	return usecase.NewExpireUnpaid(f, tenants, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

var _ workerruntime.Job = (*usecase.ExpireUnpaid)(nil)

// HU-WRK-001 AC1: it asks for the due deadlines and triggers the expiry of each reservation.
func TestHU_WRK_001_AC1_triggers_the_expiry_of_every_due_reservation(t *testing.T) {
	f := &fake{due: map[string][]out.Deadline{tenantA: deadlines("r1", "r2")}}

	processed, failed := job(f, tenantA).Run(context.Background(), 100)

	if processed != 2 || failed != 0 {
		t.Fatalf("processed %d failed %d, want 2 and 0", processed, failed)
	}
	if len(f.calls) != 2 || f.calls[0].reservation != "r1" || f.calls[1].reservation != "r2" || f.calls[0].tenant != tenantA {
		t.Errorf("calls = %+v", f.calls)
	}
}

// Annex D: the run asks for exactly BATCH_SIZE, at most the 100 the API gives in a page.
func TestHU_WRK_001_AC1_asks_for_exactly_the_batch_size_up_to_the_page_limit(t *testing.T) {
	f := &fake{}
	job(f, tenantA).Run(context.Background(), 7)
	job(f, tenantA).Run(context.Background(), 500)

	if len(f.limits) != 2 || f.limits[0] != 7 || f.limits[1] != 100 {
		t.Errorf("limits = %v, want [7 100]", f.limits)
	}
}

// Annex D: a failed item is counted and logged and the batch goes on.
func TestHU_WRK_001_AC1_a_failed_item_does_not_stop_the_batch(t *testing.T) {
	f := &fake{due: map[string][]out.Deadline{tenantA: deadlines("r1", "r2", "r3")}}
	f.expireBy = func(r string) (out.Outcome, error) {
		if r == "r2" {
			return out.Outcome{}, errors.New("boom")
		}
		return out.Outcome{Expired: true}, nil
	}

	processed, failed := job(f, tenantA).Run(context.Background(), 100)

	if processed != 2 || failed != 1 || len(f.calls) != 3 {
		t.Fatalf("processed %d failed %d calls %d, want 2, 1 and 3", processed, failed, len(f.calls))
	}
}

// The service decides: a reservation it does not expire (paid, proof in validation, frozen, tenant inactive) is an answer, not a failure.
func TestHU_WRK_001_AC3_a_reservation_the_service_does_not_expire_is_not_a_failure(t *testing.T) {
	f := &fake{due: map[string][]out.Deadline{tenantA: deadlines("r1")}}
	f.expireBy = func(string) (out.Outcome, error) { return out.Outcome{Expired: false, Reason: "TENANT_INACTIVE"}, nil }

	processed, failed := job(f, tenantA).Run(context.Background(), 100)

	if processed != 1 || failed != 0 {
		t.Fatalf("processed %d failed %d, want 1 and 0", processed, failed)
	}
}

// A tenant whose list fails is counted once and the next tenant is still swept.
func TestHU_WRK_001_AC1_a_tenant_that_cannot_be_listed_does_not_stop_the_others(t *testing.T) {
	f := &fake{due: map[string][]out.Deadline{tenantB: deadlines("r9")}, listErr: map[string]error{tenantA: errors.New("down")}}

	processed, failed := job(f, tenantA, tenantB).Run(context.Background(), 100)

	if processed != 1 || failed != 1 || len(f.calls) != 1 || f.calls[0].tenant != tenantB {
		t.Fatalf("processed %d failed %d calls %+v", processed, failed, f.calls)
	}
}

// Safe to run twice: the key of one reservation is stable inside a run (a retry repeats it) and different in the next run (the
// service stores the answer of a key, so a reservation that was not expirable must be asked again later).
func TestHU_WRK_001_AC1_the_idempotency_key_is_stable_in_a_run_and_new_in_the_next(t *testing.T) {
	f := &fake{due: map[string][]out.Deadline{tenantA: deadlines("r1")}}
	j := job(f, tenantA)

	j.Run(correlation.WithID(context.Background(), "run-one-0000"), 100)
	j.Run(correlation.WithID(context.Background(), "run-one-0000"), 100)
	j.Run(correlation.WithID(context.Background(), "run-two-0000"), 100)

	if f.calls[0].key != f.calls[1].key {
		t.Errorf("same run, keys %q and %q differ", f.calls[0].key, f.calls[1].key)
	}
	if f.calls[0].key == f.calls[2].key {
		t.Errorf("another run reused the key %q", f.calls[0].key)
	}
	if k := f.calls[0].key; len(k) < 8 || len(k) > 128 || !strings.Contains(k, "r1") {
		t.Errorf("key %q must be 8 to 128 characters and name the reservation", k)
	}
}

// Annex D: a run stops when RUN_TIMEOUT expires.
func TestHU_WRK_001_AC1_a_done_context_stops_the_run(t *testing.T) {
	f := &fake{due: map[string][]out.Deadline{tenantA: deadlines("r1", "r2"), tenantB: deadlines("r3")}}
	ctx, cancel := context.WithCancel(context.Background())
	f.expireBy = func(string) (out.Outcome, error) { cancel(); return out.Outcome{Expired: true}, nil }

	processed, _ := job(f, tenantA, tenantB).Run(ctx, 100)

	if processed != 1 || len(f.calls) != 1 {
		t.Fatalf("processed %d calls %d, want the run to stop after the first item", processed, len(f.calls))
	}
}
