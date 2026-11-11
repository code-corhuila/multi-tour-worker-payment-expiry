package http

import (
	"context"
	"fmt"

	"github.com/code-corhuila/multi-tour-worker-runtime/httpclient"

	"github.com/code-corhuila/multi-tour-worker-payment-expiry/internal/application/port/out"
)

const (
	tenantsPath = "/api/v1/commercial-reservation/internal/tenants?status=ACTIVE&page=%d&limit=%d"
	tenantsPage = 100
	maxPages    = 1000 // a guard against a service that never stops paging
)

// Tenants reads the active tenants from the internal route of commercial-reservation, which answers ids and nothing else.
type Tenants struct {
	client *httpclient.Client
}

var _ out.Tenants = (*Tenants)(nil)

// NewTenants returns the adapter over a client whose base URL is commercial-reservation.
func NewTenants(client *httpclient.Client) *Tenants { return &Tenants{client: client} }

type tenantItem struct {
	TenantID string `json:"tenantId"`
}

// ListActive reads every page of GET .../internal/tenants?status=ACTIVE.
func (t *Tenants) ListActive(ctx context.Context) ([]string, error) {
	var ids []string
	for page := 1; page <= maxPages; page++ {
		got, err := httpclient.List[tenantItem](ctx, t.client, fmt.Sprintf(tenantsPath, page, tenantsPage))
		if err != nil {
			return nil, fmt.Errorf("list the active tenants: %w", err)
		}
		for _, item := range got.Data {
			if !uuid.MatchString(item.TenantID) {
				return nil, fmt.Errorf("list the active tenants: an item has no tenant uuid")
			}
			ids = append(ids, item.TenantID)
		}
		if page >= got.Meta.TotalPages {
			return ids, nil
		}
	}
	return nil, fmt.Errorf("list the active tenants: more than %d pages", maxPages)
}
