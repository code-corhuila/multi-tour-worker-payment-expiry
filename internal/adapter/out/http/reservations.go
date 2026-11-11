// Package http is the output adapter that calls commercial-reservation on its internal routes.
package http

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/code-corhuila/multi-tour-worker-runtime/httpclient"

	"github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/application/port/out"
)

const (
	listPath   = "/api/v1/commercial-reservation/internal/tenants/%s/payment-deadlines?due=true&page=1&limit=%d"
	expiryPath = "/api/v1/commercial-reservation/internal/tenants/%s/reservations/%s/expiry"
)

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Reservations calls commercial-reservation through the client of the runtime.
type Reservations struct {
	client *httpclient.Client
}

var _ out.Reservations = (*Reservations)(nil)

// NewReservations returns the adapter over a client whose base URL is commercial-reservation.
func NewReservations(client *httpclient.Client) *Reservations { return &Reservations{client: client} }

type deadlineItem struct {
	ReservationID string `json:"reservationId"`
}

type expiryResult struct {
	Expired bool   `json:"expired"`
	Reason  string `json:"reason"`
}

// ListDue reads the first page of GET .../payment-deadlines?due=true.
func (r *Reservations) ListDue(ctx context.Context, tenantID string, limit int) ([]out.Deadline, error) {
	if !uuid.MatchString(tenantID) || limit < 1 {
		return nil, fmt.Errorf("a tenant uuid and a positive limit are required")
	}
	got, err := httpclient.List[deadlineItem](ctx, r.client, fmt.Sprintf(listPath, tenantID, limit))
	if err != nil {
		return nil, fmt.Errorf("list the due deadlines: %w", err)
	}
	due := make([]out.Deadline, 0, len(got.Data))
	for _, item := range got.Data {
		if !uuid.MatchString(item.ReservationID) {
			return nil, fmt.Errorf("list the due deadlines: an item has no reservation uuid")
		}
		due = append(due, out.Deadline{ReservationID: item.ReservationID})
	}
	return due, nil
}

// Expire is a POST with an Idempotency-Key, so the runtime may repeat it after a network failure, a 429 or a 5xx.
func (r *Reservations) Expire(ctx context.Context, tenantID, reservationID, idempotencyKey string) (out.Outcome, error) {
	if !uuid.MatchString(tenantID) || !uuid.MatchString(reservationID) {
		return out.Outcome{}, fmt.Errorf("a tenant uuid and a reservation uuid are required")
	}
	var result expiryResult
	err := r.client.Do(ctx, httpclient.Request{
		Method:         http.MethodPost,
		Path:           fmt.Sprintf(expiryPath, tenantID, reservationID),
		IdempotencyKey: idempotencyKey,
	}, &result)
	if err != nil {
		return out.Outcome{}, fmt.Errorf("expire the reservation: %w", err)
	}
	return out.Outcome{Expired: result.Expired, Reason: result.Reason}, nil
}
