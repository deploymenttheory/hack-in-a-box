# Foundation batch verification

This batch implements the ten approved foundation tickets on top of #22. The ADRs and glossary are included unchanged from the agreed domain review. Event registration, teams, challenge scheduling, submission and assessment remain separate domain work.

| Issue | Implementation | Verification |
| --- | --- | --- |
| #24 | pgxpool, five Goose migrations, sqlc queries, shared transactions and coordinated River migrations | Fresh install, earlier fixture upgrade, three concurrent migrators, incompatible schema, stale revision and rollback of state/outbox intent. |
| #23 | Vite/React shell, TanStack navigation/cache, RHF/Zod forms, Base UI/shadcn and scoped Tailwind themes | Production build; desktop/mobile deep links, invalid form, unknown route, portal scope and expired session; Go asset/API route boundaries. |
| #25 | OpenAPI 3.0, strict generated Go handlers, request validation and typed browser client | Generated response validation through the real router; 401/403/400 errors, bounded pagination, file responses and revision/idempotency conflicts. |
| #26 | Non-root API/worker image, PostgreSQL, explicit migrator and persistent volumes | Fresh Compose startup, UID 10001, health checks, deep links and restart persistence for both records and files. |
| #31 | Go/frontend/browser/integration/generation/image workflows with pinned tools/actions/images | Workflow-equivalent local commands; deliberate lint and generated-code drift rejection checks. |
| #29 | Append-only audit records, typed safe references and policy-gated query/export | Rollback, prohibited updates, event isolation, unknown action/reason rejection and denied unauthorized queries. |
| #32 | OIDC PKCE/state/nonce, issuer+subject identity, PostgreSQL SCS sessions, CSRF and bootstrap administration | Local signed-token fixture; invalid nonce/audience, one-use state replay, PKCE verification, Secure/HttpOnly/SameSite cookies, CSRF, logout, revocation and active-status checks. |
| #27 | River InsertTx plus durable approvals, identity, attempts, guarded dispatch and reconciliation | Two consuming clients, producer restart boundary, duplicate enqueue, atomic job rollback, expired lease recovery, approval/cancellation and delivered/retryable/permanent/ambiguous outcomes. |
| #28 | Local opaque storage, checksums, policy-gated download and quota-serialized finalization | Traversal, unsupported content, interrupted/oversized uploads, simultaneous quota use, idempotency, ownership, missing objects and attachment headers. |
| #30 | Dependency-independent liveness, readiness, protected metrics and safe logs | Database outage, stalled approved operation, metrics authorization, identifier-free metrics and secret-bearing request log checks. |

## Local commands

```sh
go mod verify
go build ./cmd/... ./internal/...
go vet ./cmd/... ./internal/...
(cd cmd/api && go tool govulncheck -scan=module)
HACKBOX_INTEGRATION=1 go test -race -timeout 5m ./cmd/... ./internal/...
golangci-lint run ./cmd/... ./internal/...
npm --prefix web ci
npm --prefix web run lint
npm --prefix web run build
npm --prefix web test
npm --prefix web run test:e2e
sh scripts/check-generated.sh
sh scripts/smoke-compose.sh
git diff --check
```

PostgreSQL tests use isolated Testcontainers and synthetic data. Browser checks run against the production Vite build with deterministic API fixtures; Go HTTP tests separately verify production asset routing and generated API responses. The Docker smoke test exercises the real image and persistent volumes.

## Review boundaries

- The original foundation used a restrictive administrator policy. Event-role authorization now replaces it; see [organiser verification](organiser-verification.md) for current grants and policy coverage.
- The worker has no real external integration handler yet. The execution/reconciliation protocol is tested with deterministic providers; future handlers must respect cancellation, recheck eligibility and reconcile uncertain outcomes.
- OIDC must be configured before real sign-in. No fake login exists in production or development.
- Run explicit migrations before starting either process. Take a matching database/file-volume backup before upgrades.
