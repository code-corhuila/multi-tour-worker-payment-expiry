# multi-tour-worker-payment-expiry

> Worker job 1: payment expiry (commercial-reservation)

Part of the **Multi Tour** distributed system — team `multi-tour`, Grupo 2.
Governance and documentation live in [`multi-tour-docs`](https://github.com/code-corhuila/multi-tour-docs).

## What it is

Job 1 of ADR-004 D3 (HU-WRK-001). Every minute, for each tenant it sweeps, it asks `commercial-reservation` for the payment, balance and difference
deadlines that were reached (`GET .../internal/tenants/{tenantId}/payment-deadlines?due=true`) and asks it to expire each reservation
(`POST .../reservations/{reservationId}/expiry`, with an `Idempotency-Key`). The service decides: it cancels for non-payment and releases the seat, and
it refuses by itself when the departure started, a proof is in validation, the deadline is frozen or the tenant is inactive (HU-COM-011, HU-COM-020). The
worker holds no deadline rule, has no database and exposes only `/health`.

## How to start it

Copy `.env.example`, fill `SERVICE_TOKEN` (never versioned) and, optionally, `TENANT_IDS`, and run `docker compose -f deploy/compose.yml up`. The configuration is read at
start and a bad variable stops the worker naming it.

## How to test it

`gofmt -l .` must print nothing, then `go vet ./...` and `go test ./...` (Go 1.26). The same run in `ci.yml` on pull requests.

## What is missing

- The tenants: when `TENANT_IDS` is empty each run asks commercial-reservation for the active tenants (`GET /internal/tenants?status=ACTIVE`, the `SERVICE` token). `TENANT_IDS` stays as the fallback and wins when set (declared debt until the identity-audit route exists, HU-WRK-001 DP-08).
  How the job lists the tenants is open (DP-08 of the spec of HU-COM-011).
- Tested against a stub of the contract, not against the running api.

## Branching

Three permanent branches. **None of them accepts a direct commit** — you enter through a child
branch and leave through a Pull Request.

```
develop  <--PR--  feat/... fix/... chore/...
qa       <--PR--  qa/...
main     <--PR--  release/...  hotfix/...
```

Promotion happens **by re-application** (`git cherry-pick -x`), never by merging one permanent
branch into another: `merge develop -> qa` and `merge qa -> main` do not exist in this model.

`main` requires **1 approval from `ariel5253`**. On `develop` and `qa` the team sets its own review
rule.

Full policy: `00-governance/branching-policy.md` in `multi-tour-docs`.
