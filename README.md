# Hack in a Box

A self-hosted hackathon platform: a Go API and durable worker, a React/TypeScript browser application, PostgreSQL and private persistent-volume file storage. The product vocabulary is in [GLOSSARY.md](GLOSSARY.md), and the agreed rules are in [docs/adr](docs/adr).

The foundation implements database migrations, the application shell, generated REST contracts, OIDC sessions, audit records, durable delivery operations, protected files and operational checks. Versioned registration forms, FIFO allocation and offers, organiser applicant review, and published/frozen team membership now supply attendance and team lifecycle facts. Challenge publication, team suggestions, mentor assignment and judging remain in their domain tickets. See [preparation verification](docs/preparation-verification.md) for the implementation sequence and tested boundaries. Event-resource access uses current event memberships and resource assignments. Platform administration manages ownership without implicitly granting private content, credentials or assessments. See [organiser verification](docs/organiser-verification.md).

## Run with Docker Compose

Prerequisite: Docker with Compose v2 and OpenSSL on the host.

```sh
sh scripts/init-env.sh
docker compose build
docker compose up -d --wait postgres
docker compose run --rm migrate
docker compose up -d --wait api worker
```

Open <http://127.0.0.1:8080>. The initial page and health checks work without a provider; signing in requires configuring OIDC in `.env`. There is no development authentication bypass.

`init-env.sh` creates a private `.env` with random database and metrics credentials. It preserves an existing file. Credentials are supplied at runtime and are excluded from image builds. Database and file volumes survive container replacement. API and worker run as UID/GID 10001 with a read-only root filesystem. PostgreSQL is available only on the Compose network. Bind a reverse proxy to the loopback API port for HTTPS deployments.

For an upgrade, back up the database and file volume together, stop API/worker, build the new image, run the explicit `migrate` step, and restart. Migrations serialize across processes and apply the embedded Goose migrations and the pinned River schema. Both processes refuse incompatible application or River schema versions. Migration failure prevents startup; restore the matching backup/image if an upgrade cannot be completed. The application never runs migrations implicitly.

## Configure sign-in

Set `HACKBOX_PUBLIC_ORIGIN`, `HACKBOX_OIDC_ISSUER`, `HACKBOX_OIDC_CLIENT_ID` and, for a confidential client, `HACKBOX_OIDC_CLIENT_SECRET`. Register `<public-origin>/auth/callback` with the provider. For Entra ID use the exact tenant-specific v2 issuer, not a common/multi-tenant authority. Identity is keyed by issuer and subject; email never links accounts.

Set `HACKBOX_BOOTSTRAP_SUBJECT` to the stable subject of the first administrator. Sign in once, then remove this setting and restart so future logins do not regrant administration. Bootstrap grants are audited. Production requires an HTTPS origin, HTTPS issuer, client ID and a metrics token of at least 32 characters. Production cookies are Secure, HttpOnly and SameSite=Lax. Configure the provider and reverse proxy before switching `HACKBOX_ENV` to `production`.

OIDC uses authorization code flow with PKCE, one-use browser-bound state and a nonce. SCS sessions live in PostgreSQL; the database stores hashes of cookie tokens. Authentication renews the session token. Every protected request reloads the person's active status and session revision. Privilege changes must increment `people.session_revision` in the same transaction as the grant/revocation, forcing existing sessions to sign in again. Logout invalidates the server session. Unsafe API requests require the current session's `X-CSRF-Token` and an `Origin` exactly matching `HACKBOX_PUBLIC_ORIGIN`.

## Develop locally

Use Go 1.27.3+, Node 22.23.3 and Docker for PostgreSQL tests. Set `HACKBOX_ENV=development` and export `HACKBOX_DATABASE_URL` for an explicitly provisioned development database. Go commands read environment variables, not `.env` files. Compose derives its own connection string from `.env`.

```sh
go mod download
npm --prefix web ci
npm --prefix web run build
go run ./cmd/migrate
go run ./cmd/api
# In another terminal with the same configuration:
go run ./cmd/worker
```

For browser hot reload, run `npm --prefix web run dev`; Vite proxies `/api` and `/auth` to `127.0.0.1:8080`. Set the public origin and provider redirect URI to the Vite origin when testing login there. Production assets are served by Go from `HACKBOX_WEB_DIR` (default `web/dist`); browser deep links fall back to the application while `/api`, `/auth` and `/ops` retain their own routing errors.

Key settings beyond authentication are:

| Variable | Default / rule |
| --- | --- |
| `HACKBOX_ENV` | Required: `development`, `test`, `production` |
| `HACKBOX_DATABASE_URL` | Required by Go processes; explicit PostgreSQL connection string |
| `HACKBOX_HTTP_ADDR` | `127.0.0.1:8080`; Compose uses `0.0.0.0:8080` internally |
| `HACKBOX_LOG_LEVEL` | `INFO` |
| `HACKBOX_MAX_BODY_BYTES` | 16 MiB; positive, at most 64 MiB |
| `HACKBOX_REQUEST_TIMEOUT` | `10s`, at most `5m` |
| `HACKBOX_SHUTDOWN_TIMEOUT` | `20s`, at most `5m` |
| `HACKBOX_STORAGE_PATH` | `data/files`; image uses `/data/files` |
| `HACKBOX_FILE_MAX_BYTES` | 8 MiB; at most 32 MiB |
| `HACKBOX_FILE_EVENT_QUOTA` | 1 GiB; at least the upload limit |
| `HACKBOX_METRICS_TOKEN` | Required for metrics access; minimum 32 characters |

The file API accepts bounded JSON/base64 uploads. Keep the request-body limit above the base64-encoded upload limit plus JSON overhead. PDF, PNG, JPEG and plain text are checked against their detected content type. Files have opaque keys, checksums and ownership metadata; downloads require resource authorization and use attachment headers. Event locking serializes quota checks. Identical retries reuse the same file; a conflicting idempotency key or stale event revision returns 409. Unfinished writes are removed immediately; crash leftovers older than 24 hours are reconciled against metadata by the worker. A missing or corrupt object returns an actionable error and must be restored from the matching file-volume backup.

## Contracts and boundaries

[api/openapi.yaml](api/openapi.yaml) is the OpenAPI 3.0 source of truth. Go strict Chi interfaces/models and TypeScript types are generated; the browser uses `openapi-fetch`. Request validation complements authentication and resource policy. Errors have a safe `code`, `message` and server-generated `request_id`. Pagination is bounded; action endpoints use idempotency keys and quoted `If-Match` revisions.

```sh
sh scripts/generate.sh
sh scripts/check-generated.sh
```

Go tool directives and `go.sum` pin sqlc and oapi-codegen; `web/package-lock.json` pins browser tools. Commit generated outputs with their sources. sqlc queries can be bound to the same pgx transaction as domain mutations, audit records and River `InsertTx` calls via `database.Within`.

`internal/app` wires dependencies. `internal/access` owns the policy boundary; `identity` authenticates. `audit` accepts defined action/reason codes and typed references instead of arbitrary confidential text. `files` adapts private storage. `jobs` owns durable operation identity, approvals, attempts and reconciliation independently of River retention. Domain modules must not import HTTP adapters or the composition root.

Future integration handlers implement `Eligible`, `Deliver` and `Reconcile`; all receive a stable operation ID for provider idempotency. No external provider handler is installed in this foundation. Due time does not approve an operation. Workers recheck approval, cancellation and current eligibility before delivery. Database guards prevent simultaneous ownership, and expired attempts are reconciled. Outcomes are delivered, retryable, permanent or ambiguous; unknown outcomes become ambiguous and are never blindly retried. Reconciliation runs at startup and every minute from persisted records. See [operations](docs/operations.md) before implementing a provider.

The React shell uses TanStack Router/Query, React Hook Form/Zod and shadcn Base UI components with Tailwind. Private query keys include person and event; changing accounts destroys the private cache. Portals stay inside the event theme scope. Form schemas improve feedback; the server remains authoritative.

## Verify

```sh
go build ./cmd/... ./internal/...
go vet ./cmd/... ./internal/...
(cd cmd/api && go tool govulncheck -scan=module)
HACKBOX_INTEGRATION=1 go test -race -timeout 5m ./cmd/... ./internal/...
golangci-lint run ./cmd/... ./internal/...
npm --prefix web run lint
npm --prefix web run build
npm --prefix web test
npx --prefix web playwright install chromium
npm --prefix web run test:e2e
sh scripts/check-generated.sh
sh scripts/smoke-compose.sh
git diff --check
```

Without `HACKBOX_INTEGRATION=1`, database integration tests explicitly skip. With it set, Testcontainers starts isolated, version-pinned PostgreSQL databases and missing Docker fails the run. OIDC fixtures are local; no organisation/provider credentials are required. Browser tests cover desktop and narrow screens. `smoke-compose.sh` creates and removes only its uniquely named synthetic installation, including its test volumes; it uses port 18080.

CI runs these checks, dependency review and container verification with pinned actions and read-only repository permissions. Application Go commands target `cmd` and `internal` so installed browser tooling cannot become an accidental Go package. Operational endpoints, alert thresholds and failure handling are documented in [docs/operations.md](docs/operations.md).

Verified GitHub linking is separate from organisation sign-in. See
[GitHub identity setup and verification](docs/github-identity.md) for the optional
OAuth configuration, encrypted token key and invitation-review behaviour.
