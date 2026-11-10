// Package out holds the ports the job needs from the outside world.
package out

import "context"

// Deadline is one reservation whose payment deadline was reached, as the list names it. The service re-checks it at the expiry.
type Deadline struct {
	ReservationID string
}

// Outcome is what the service decided when asked to expire a reservation.
type Outcome struct {
	Expired bool
	// Reason says why it was not expired (NOT_DUE, PAID, PROOF_IN_VALIDATION, FROZEN, ALREADY_CANCELLED, TENANT_INACTIVE).
	Reason string
}

// Reservations is commercial-reservation as the job sees it: the application never learns that it is HTTP.
type Reservations interface {
	// ListDue reads the first limit deadlines that are due in a tenant. The service lists none while the tenant is inactive.
	ListDue(ctx context.Context, tenantID string, limit int) ([]Deadline, error)
	// Expire asks the service to cancel one reservation for non-payment. The service decides; it is safe to repeat with the same key.
	Expire(ctx context.Context, tenantID, reservationID, idempotencyKey string) (Outcome, error)
}
