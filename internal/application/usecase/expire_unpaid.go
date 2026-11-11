// Package usecase holds the jobs of the worker.
package usecase

import (
	"context"
	"fmt"
	"log/slog"

	workerruntime "github.com/code-corhuila/multi-tour-worker-runtime"
	"github.com/code-corhuila/multi-tour-worker-runtime/correlation"

	"github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/application/port/out"
)

// maxPageSize is the most the list of the contract gives in one page.
const maxPageSize = 100

// ExpireUnpaid is job 1: per tenant it asks the service for the payment deadlines that were reached and asks it to expire
// each reservation. The service decides what is expirable (the departure start, a proof in validation, a frozen deadline and an
// inactive tenant are its rules, HU-COM-011, HU-COM-020), so the job holds no deadline logic.
type ExpireUnpaid struct {
	reservations out.Reservations
	tenants      []string
	source       out.Tenants
	log          *slog.Logger
}

var _ workerruntime.Job = (*ExpireUnpaid)(nil)

// NewExpireUnpaid wires the job over the tenants it sweeps.
func NewExpireUnpaid(reservations out.Reservations, tenants []string, log *slog.Logger) *ExpireUnpaid {
	return &ExpireUnpaid{reservations: reservations, tenants: tenants, log: log}
}

// Name is how the runner logs the job.
func (*ExpireUnpaid) Name() string { return "payment-expiry" }

// Run takes at most batch due reservations of each tenant (the list is capped at the page limit). What it expires leaves the list,
// so what does not fit waits for the next run.
func (j *ExpireUnpaid) Run(ctx context.Context, batch int) (processed, failed int) {
	tenants, ok := j.sweepTenants(ctx)
	if !ok {
		return 0, 1
	}
	for _, tenant := range tenants {
		if ctx.Err() != nil {
			break
		}
		due, err := j.reservations.ListDue(ctx, tenant, min(batch, maxPageSize))
		if err != nil {
			failed++
			j.log.WarnContext(ctx, "the due deadlines could not be listed, the next run asks again", "tenantId", tenant, "error", err)
			continue
		}
		done, bad := workerruntime.Each(ctx, j.log, due, func(ctx context.Context, d out.Deadline) error { return j.expire(ctx, tenant, d) })
		processed, failed = processed+done, failed+bad
		j.log.InfoContext(ctx, "payment deadlines swept", "tenantId", tenant, "listed", len(due), "handled", done, "failed", bad)
	}
	return processed, failed
}

// expire asks for the expiry. The key names the reservation and the run: a retry inside the run repeats it (the service answers
// the stored result), while a later run asks again, because the service stores the answer of a key and a reservation that was
// not expirable today may be tomorrow.
func (j *ExpireUnpaid) expire(ctx context.Context, tenant string, d out.Deadline) error {
	key := fmt.Sprintf("expiry-%s-%s", d.ReservationID, correlation.From(ctx))
	outcome, err := j.reservations.Expire(ctx, tenant, d.ReservationID, key)
	if err != nil {
		return fmt.Errorf("expire reservation %s of tenant %s: %w", d.ReservationID, tenant, err)
	}
	if outcome.Expired {
		j.log.InfoContext(ctx, "reservation expired", "tenantId", tenant, "reservationId", d.ReservationID)
		return nil
	}
	j.log.InfoContext(ctx, "reservation not expired by the service", "tenantId", tenant, "reservationId", d.ReservationID, "reason", outcome.Reason)
	return nil
}

// WithTenantSource lets the job ask for the tenants at each run when none are configured; TENANT_IDS stays the fallback and wins when set.
func (j *ExpireUnpaid) WithTenantSource(source out.Tenants) *ExpireUnpaid {
	j.source = source
	return j
}

// sweepTenants is the configured list, or the active tenants of the service when there is none. A list that cannot be read counts as one
// failure and sweeps nothing: the next run asks again.
func (j *ExpireUnpaid) sweepTenants(ctx context.Context) ([]string, bool) {
	if len(j.tenants) > 0 || j.source == nil {
		return j.tenants, true
	}
	ids, err := j.source.ListActive(ctx)
	if err != nil {
		j.log.WarnContext(ctx, "the active tenants could not be listed, the next run asks again", "error", err)
		return nil, false
	}
	return ids, true
}
