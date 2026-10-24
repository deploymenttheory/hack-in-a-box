# Interactive style guide

The guide imports production Tailwind v4 tokens, Lucide registry, primitives and
collection/detail/review/action-queue patterns. It is a separate static output,
not an authenticated application route. Every person, event, note, request and
receipt in its examples is synthetic; scenario state lives in mounted memory.
No browser API, cookie, private store, identity provider or database is needed.

## Browse independently

From the repository root, with Docker Desktop running:

```sh
docker compose -f compose.styleguide.yaml --profile styleguide up -d --build --wait
```

Open <http://127.0.0.1:8082>. This uses its own `hackbox-styleguide` project and
network with one service, without application/database/worker dependencies or
volumes. It works while either portal is unavailable. No `.env` initialization,
OIDC settings or database password is required. Stop this service with:

```sh
docker compose -f compose.styleguide.yaml --profile styleguide down
```

`HACKBOX_STYLEGUIDE_PORT` changes the local host port;
`HACKBOX_STYLEGUIDE_IMAGE` changes the image tag;
`HACKBOX_STYLEGUIDE_REVISION` embeds a commit identifier during the build.
The non-root read-only service serves only guide documents and static assets.
`/healthz` checks the host; `/readyz` requires all three static entry documents.
It denies `/api`, `/auth` and application routes, and its CSP denies connections.
The only allowed frames are its own component/task examples.

Without Docker:

```sh
npm --prefix web ci
npm --prefix web run build:styleguide
HACKBOX_WEB_DIR=web/dist-styleguide HACKBOX_HTTP_ADDR=127.0.0.1:8082 go run ./cmd/styleguide
```

`npm --prefix web run preview:styleguide` is also a static Vite preview, not the
production Go host. Browser checks use the actual Go host, deny cookie/private
storage access, observe requests, and fail on console/runtime errors.

## Publish a versioned artifact

The `styleguide` CI job builds, exercises and uploads `styleguide-<full-head-sha>`
for every successful head. Download that workflow artifact and extract its
contents at the root of an approved docs origin, or build Docker's `styleguide`
target. Retain `index.html`, `samples.html`, `patterns.html`, `assets/` and the
Inter license together. Serve correct MIME types, no-store HTML, immutable
fingerprinted assets and the same CSP/security headers as `internal/styleguide`.
The guide currently assumes an origin root; a path-prefix mount needs deliberate
base/entry routing changes and tests. External hosting, DNS and credentials are
not provisioned by this feature. Do not use production portal cookies on the docs
origin. Promote the artifact corresponding to the production component version.

Both portal footers expose “About this interface · Style guide”. The single public
build setting `VITE_STYLEGUIDE_URL` defaults to `http://127.0.0.1:8082`. Set it to
the approved HTTP(S) docs origin before building both portals, for example:

```sh
VITE_STYLEGUIDE_URL=https://docs.example.test docker compose build api admin
```

Compose passes the same setting as a Docker build argument to each portal.
Changing it requires rebuilding; setting a runtime environment variable alone
does not rewrite static HTML. URLs with credentials or executable schemes fail.
Do not put secrets in this or any `VITE_` build setting.

## Use and extend the system

Start with [the visual contract](visual-contract.md), [primitives](primitives.md)
and [patterns](patterns.md). The guide's Components section exercises the actual
controls; Task patterns exercises prepare → review/correct/cancel → exact outcome
→ next action. Use the labelled synthetic transport control for empty, delayed,
rejected, stale, read-failure, unknown and latest-only responses. Unknown outcomes
permit a status read without another write. Reset clears all mounted example
state; changing account/event/portal drops the corresponding synthetic scope.
Appearance and density changes retain drafts. Viewport controls resize a genuine
iframe viewport; available width can constrain a requested size on small screens.

Task patterns also imports the actual production `ProtectedUploadControls` shared
by content attachments, `TaskEvidence` and team deliverables. Its owner-controlled
example chooses synthetic file metadata, captures one key/revision/team/reason,
permits deliberate identical retries after an unknown callback, clears metadata
on denial and focuses the relevant feedback. A synthetic owner callback is not a
file upload, attachment acceptance or formal submission receipt. The example has
no FileReader, byte transfer, account or API transport; it can display selected
local metadata without reading the file. Reset restores the labelled native
input. Usage/reference entries explain owner scope barriers, privacy, pending
locks, focus, and the separate native `ProtectedUpload` adapter.

Every future UI PR must update production code, its interactive examples,
copyable snippets and intended-use/variant/semantics/accessibility/anti-pattern
guidance together when behaviour changes. Keep the actual icon registry as the
catalogue source. Use fixtures from `components/patterns/scenario`, never copied
participant data. Keep examples out of production entry points; never add a fake
receipt or transport to a production pattern. Shared visual state cannot authorize
a domain command or replace generated API contracts, CSRF and version checks.

Run meaningful checks from the `web` directory:

```sh
npm run lint
npm test
npm run build
npm run test:e2e:styleguide
```

To run the same suite against the opt-in Docker publication instead:

```sh
STYLEGUIDE_TEST_ORIGIN=http://127.0.0.1:8082 npx playwright test --config playwright.styleguide.config.ts
```

The guide browser suite normally starts only its static Go host, has no API/DB service,
checks real narrow viewports and keyboard focus, and makes cookie/private-store
access fail. Existing portal journeys and full repository CI still apply. Capture
synthetic desktop/mobile visual evidence and compare it with the accepted Fleet
reference; guide verification is not certification of every future domain screen.

The media decision history example uses the production `MediaHistoryDisclosure`
and `MediaHistoryContent` with a local synthetic owner. Open starts in loading;
explicit controls supply metadata-only cursor pages, read failure or access denial.
Retry and reopen return to loading without reusing earlier rows. Close restores
focus to the mounted disclosure button; expanded/controls attributes describe the
region and the whole disclosure is hidden in print. Ready rows accept only version,
action and recorded time. Loading, failure and denial states cannot carry rows.
Use this beside a current native `can_read_history` record, including read-only
archives. The native adapter owns fresh per-mount scope, current authorization,
no-store transport and late-response fences. Never pass historical documents,
consent tokens, captions, reasons or byte URLs. History is neither a byte grant,
renewed approval nor an original command receipt. The example has no session,
private persistence or transport. Its copyable snippet documents the owner seam.
