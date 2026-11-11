# multi-tour-worker-payment-expiry

> Worker job 1: payment expiry (commercial-reservation)

Part of the **Multi Tour** distributed system — team `multi-tour`, Grupo 2.
Governance and documentation live in [`multi-tour-docs`](https://github.com/code-corhuila/multi-tour-docs).

## What it is

Job 1 of ADR-004 D3 (HU-WRK-001): it expires the reservations whose payment, balance or difference deadline was reached, by asking `commercial-reservation`
through its internal routes. It has no database and exposes only `/health`. This piece is the skeleton: module, common files, CI, configuration and health.

## How to start it

Copy `.env.example`, fill `SERVICE_TOKEN` (never versioned) and `TENANT_IDS`, and run `docker compose -f deploy/compose.yml up`. The configuration is read at
start and a bad variable stops the worker naming it.

## How to test it

`gofmt -l .` must print nothing, then `go vet ./...` and `go test ./...` (Go 1.26). The same run in `ci.yml` on pull requests.

## What is missing

The job itself (list the due deadlines and ask for each expiry) is the next piece. The tenants come from `TENANT_IDS` because no service route lists them
for a `SERVICE` token.

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
