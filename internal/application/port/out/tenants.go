package out

import "context"

// Tenants names the tenants the job sweeps when no list is configured (HU-WRK-001 DP-08): the active ones of commercial-reservation.
type Tenants interface {
	// ListActive reads every active tenant id. A failure is an error, never a partial list.
	ListActive(ctx context.Context) ([]string, error)
}
