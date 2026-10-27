# GitHub App configuration and verification

Issue #91 connects an organisation-installed GitHub App independently of participant sign-in and GitHub identity linking. This implementation performs read-only configuration verification. It does not create repositories, send invitations, run participant code or delete resources.

## Human setup and sandbox acceptance

**Live sandbox acceptance is pending.** No approved sandbox organisation, App registration/installation or App private key is configured in the current development environment. Mocked tests and CI do not satisfy that acceptance step. Keep issue #91 open until an organisation owner approves a sandbox and the read-only verification below is captured.

An organisation owner must complete these steps; the coding agent must not create or install an App on an unspecified account:

1. Identify and approve the sandbox organisation and a disposable template repository within that organisation. Record the organisation login and immutable numeric account ID. Mark the repository as a template. Do not use a production event's repositories for setup verification.
2. Open the organisation's **Settings → Developer settings → GitHub Apps → New GitHub App**, or the organisation App registration URL `https://github.com/organizations/ORGANIZATION/settings/apps/new`. Register a separate integration App, not the participant OAuth app. Set the **Setup URL** to the admin deployment's `https://ADMIN_HOST/auth/github-app/callback`. The GitHub App setup URL differs from a user-authorization OAuth callback. Do not enable user authorization during installation for this server-to-server integration. Webhooks are not consumed by this ticket; leave them disabled rather than inventing an unimplemented webhook receiver.
3. Request repository **Contents: read**, **Metadata: read** and **Administration: write** for the planned repository creation and collaborator-invitation operations. No organisation Members permission or account email permission is needed for repository collaborators. The diagnostic token requests only Metadata read on the named template; Contents read is checked as a prerequisite for the future template-creation and activity operations. Record the public App ID and App slug. Download an App private key into protected local storage; never paste it into chat, shell command arguments, logs or issue comments.
4. Inject `HACKBOX_CREDENTIAL_KEYRING` into both API deployments using the deployment secret manager as described in [team-credential-recovery.md](team-credential-recovery.md). As a current event organiser, configure the event with `POST /api/v1/events/{event_id}/github-app` using `action=configure`, `app_id`, `app_slug`, `organization_id`, `organization`, `template_repository` and the PEM in `private_key`. Submit the PEM only in the protected request body with CSRF and the observed event revision. The API stores authenticated ciphertext, never returns the PEM and does not put it in audit data. Do not place the private key in a template/resource field.
5. From that same signed-in admin browser session, call `POST .../github-app/installation-authorizations` with CSRF and the current event revision. Open its returned fixed GitHub installation URL. Install the App on the approved organisation and select the intended template repository. The browser must return to the configured admin Setup URL with the same session. The server consumes a five-minute single-use state, rechecks the organiser/event/session revision and verifies the installation with App-authenticated GitHub requests against both the configured organisation ID and login. A request to install still requires an organisation owner's approval; it is not a completed installation.
6. Read `GET .../github-app` and record only the safe diagnostics, timestamp and approved organisation/template identifiers. Expected result is `healthy`, with each endpoint permission check allowed. Exercise a non-production permission change or remove template access, refresh status and record the actionable diagnostic; restore the intended sandbox settings afterward. Do not create a repository or send an invitation merely to validate this configuration ticket. Any later write test needs its own approved sandbox action.

These are proposed manual setup stages, ready for the user to approve before a wizard is authored or any App registration/installation is performed. Public identifiers and a protected file path can be supplied to the coding agent; never supply PEM or tokens in chat. The production OIDC provider must permit the separate admin deployment's callback as already documented by the deployment split.

## API and current authority

All integration metadata, configuration, key rotation and installation initiation require a current active event organiser. Configuration writes, key rotation and installation initiation also require an event permitting writes; organisers can still inspect integration health when an event is read-only. Platform administrators and participant GitHub linking do not implicitly grant integration access. Configure and authorise operations use the observed event `If-Match` revision and CSRF. Configure resets installation binding and invalidates pending attempts. A `rotate` command re-encrypts the retained App key under the active deployment key and preserves installation binding; `disconnect` removes the app configuration and pending attempts without uninstalling the App or revoking provider resources.

The setup callback deliberately has no ordinary CSRF header: its one-use, session-bound state protects the external redirect. Session expiry, identity session-revision changes, role removal, changed event revision, expiry, replay and another browser's session all reject the callback. Failure consumes a correctly bound attempt, so start installation again. Neither callback input nor `installation_id` alone proves an installation. The server verifies the signing App's ID/slug, installation App ID, organisation type and immutable ID/login and suspension state. This proves the configured organisation/App binding; it does not claim the app identity signed into Hack-in-a-Box is the GitHub user who installed the App.

Safe GET diagnostics recheck the current installation and template through GitHub rather than trusting stale database health. Missing permissions, removed/suspended installation, wrong App/organisation, inaccessible/invalid template, key failure, rate limit and provider outage produce bounded codes without provider bodies. The template must be an active template owned by the intended organisation. Missing template access never silently falls back to a public unauthenticated read.

## Endpoint permissions and transport scope

| Operation | GitHub endpoint | Required permission |
| --- | --- | --- |
| Verify signing App | `GET /app` | App JWT |
| Verify installation/account | `GET /app/installations/{id}` | App JWT |
| Mint token | `POST /app/installations/{id}/access_tokens` | App JWT; explicitly restricted repository names and operation permissions |
| Read template/metadata | `GET /repos/{owner}/{repo}` | Metadata read for the template-specific verification token |
| Create from template (future provisioner) | `POST /repos/{template_owner}/{template_repo}/generate` | Contents read plus Administration write, or Contents read plus Repository creation write |
| Invite/update repository collaborator (future provisioner) | `PUT /repos/{owner}/{repo}/collaborators/{username}` | Administration write |
| List repository invitations (future provisioner) | `GET /repos/{owner}/{repo}/invitations` | Administration read |
| Cancel repository invitation (future provisioner) | `DELETE /repos/{owner}/{repo}/invitations/{id}` | Administration write |
| Read repository commits/activity (future evidence collector) | `GET /repos/{owner}/{repo}/commits` | Contents read |

GitHub added `repository_creation:write` in October 2026, allowing repository creation without broader Administration write. This alone does **not** permit collaborator invitations. Diagnostics read the raw permission map through the official REST client so the new permission is not lost by a client enum released earlier. Because the planned managed-repository workflow also manages invitations, the setup above still requests Administration write. No organisation membership invitation is sent by this implementation.

The pinned REST client is `github.com/google/go-github/v89 v89.0.0`; the App-auth transport is `github.com/bradleyfalzon/ghinstallation/v2 v2.19.0`. That transport pins go-github/v88 internally; its v88 token-option structs remain private to the transport while the app uses v89 REST clients through `http.RoundTripper`. Auth transport refreshes short-lived tokens server-side. Tokens are never persisted or returned to browsers. Verification mints a token restricted to exactly the configured template and Metadata read only. Future provisioners must derive repository names from their current event-authorised mapping, recheck live installation permissions and request a repository/operation-specific client; an empty repository scope is rejected.

References checked against primary documentation:

- [GitHub setup URL and spoofed installation IDs](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/about-the-setup-url)
- [App authentication and installation/token endpoints](https://docs.github.com/en/rest/apps/apps)
- [Create from a template](https://docs.github.com/en/rest/repos/repos#create-a-repository-using-a-template)
- [Repository invitation permissions](https://docs.github.com/en/rest/collaborators/invitations)
- [Repository commit read permissions](https://docs.github.com/en/rest/commits/commits#list-commits)
- [Repository collaborator permissions](https://docs.github.com/en/rest/collaborators/collaborators#add-a-repository-collaborator)
- [Least-privilege repository creation permissions, October 2026](https://github.blog/changelog/2026-10-13-github-app-improvements-for-least-privilege-and-management/)
- [ghinstallation transport source and pinned dependency](https://github.com/bradleyfalzon/ghinstallation/tree/v2.19.0)

## Key recovery

App private keys use the same deployment keyring utility as supplied team credentials, with a distinct authenticated context bound to the event, App ID and credential version. App credentials have their own organiser policy and never use the team-reveal API. Retain old deployment keys until all app ciphertext and retained backups have been rotated. Restore an accidentally removed deployment key under its original identifier. If the App key is revoked at GitHub, generate a replacement at GitHub and submit a new `configure` command; installation must be revalidated. Disconnecting the app locally does not revoke already issued provider tokens, remove collaborators or uninstall the provider App. Apply provider revocations separately when required.

## Signed repository activity

Configure the GitHub App's single webhook URL as
`https://<workspace-origin>/api/v1/integrations/github/webhook`, with JSON payloads
and a high entropy webhook secret. The `push` subscription requires repository
Contents read access (already covered by the base App's Contents write scope).
The optional `member` subscription additionally requires organisation Members
read access. This extra permission belongs to the webhook subscription; the
base repository collaborator API does not require organisation Members access.
See GitHub's [push](https://docs.github.com/en/webhooks/webhook-events-and-payloads#push)
and [member](https://docs.github.com/en/webhooks/webhook-events-and-payloads#member)
subscription requirements.

The `member` event with action `added` records acceptance of a repository
invitation; this is selected operational evidence, not the invitation management
state owned by #93. Without the optional Members permission/subscription, no
member deliveries are expected and the webhook acceptance count is incomplete.
Use #93's invitation/collaborator polling path for acceptance reconciliation
instead. This implementation does not change external App permissions.

Supply the same App webhook secret as the write-only `webhook_secret` field on
an authorised `configure` command to the event's existing GitHub App endpoint.
It must contain 32–256 bytes. The server encrypts it under the credential keyring
with a separate event/App/version purpose from App signing keys and participant
identity tokens. Status returns only `webhook_configured`; secret values are
never returned. Omitting the field retains the secret for the same App; replacing
the App clears an omitted secret, and disconnect removes its encrypted copy.
The existing `rotate` command re-encrypts the retained secret with the active
local key; it does not change GitHub's shared secret or make provider calls.

GitHub has one App-level webhook secret. Event-bound encrypted copies for an App
must correspond to that configured secret. When rotating the shared secret in
GitHub, update every relevant event configuration explicitly. This service never
changes GitHub's webhook configuration. Replacing or locally re-encrypting an
event secret invalidates pending deliveries authenticated with its old version;
operators can see their discarded disposition. New deliveries must authenticate
with the current secret.

The machine endpoint has no browser session or CSRF dependency. It reads only an
untrusted numeric repository ID to route through #92's immutable registry, then
checks HMAC-SHA256 over the exact original body before selecting or writing
activity. It accepts at most 1 MiB, rejects unsupported content encoding/events,
checks the current App/installation/organisation/repository identity, and admits
at most 1,000 pending deliveries per event. At capacity it returns 503 with a
30-second `Retry-After`; GitHub redelivery is explicit. A delivery GUID identifies
one original payload, and ingestion and durable job creation share a transaction.

`GET /api/v1/events/{event_id}/github-activity` requires current organiser access
and returns additive informational totals plus the latest 100 delivery records,
operation IDs, attempts, approval/cancellation, job state and recorded/discarded
status. The existing durable worker/reconciliation loop retries local failures
and recovers interrupted application transactions without double counting. A
read-only event retains readable evidence and rejects further webhook mutation;
work waits under the existing job policy while writes are disabled. A worker
rechecks approval, current source identity, secret version, team and repository
state before applying selected metadata. No payload, commit message, private
repository description, credential or webhook signature is logged or stored.
Member deliveries have no provider occurrence timestamp, so `provider_at` remains
null; push provider time uses repository `pushed_at`, never commit authored time.

Push activity is admitted after actual challenge publication, including before
live start. Delayed activity cannot lower the latest provider time, overwrite a
branch head or change repository readiness. Counts are operational evidence only:
webhooks never create formal submissions, alter assessment scores, reactivate a
disbanded competition entry, delete repositories or execute attendee code.

Primary references checked against the installed `go-github/v89` event types:
[GitHub signature validation](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries)
and [GitHub webhook events and payloads](https://docs.github.com/en/webhooks/webhook-events-and-payloads).

## Webhook migration rollback

Migration 46 rollback is allowed only while its webhook tables are empty and no
`github_webhook_activity` durable operations exist. Configured secrets, delivery
GUID/digest/provenance, selected metadata, activity and associated work prevent
rollback. An empty migration 46 rollback preserves retained schema 45 repository and
provisioning history. The migration does not purge retained evidence.

## Native repository provisioning

The worker records one `repository.provision` operation per actual published team, including when the App is configured after allocation publication. Draft allocations never provision. An organiser can also queue the same idempotent batch at `POST /api/v1/events/{event_id}/repositories` with `{"action":"queue"}`, CSRF and the current quoted event revision. `GET` returns separate pending/ready/review/ambiguous results for each team. Currently confirmed team members can read only their own mapping after actual challenge publication, including during preparation before live start. Provisioning never creates a submission, accepts competition output or awards a score. Invitation/membership management remains a separate integration responsibility.

The repository name contains both complete event and team UUIDs. The application-owned `team_repositories` registry snapshots the App, installation, organisation, template, numeric template ID and a random provenance marker. Before the first provider create, the worker commits an attempted flag and template identity. It requests a private repository from exactly that template, with the provenance marker as its description and an operation-specific token restricted to the template. Recovery uses a metadata-only token restricted to the deterministic target name. Neither token nor the PEM is returned to callers.

A pre-existing name is a collision requiring review, even if other metadata resembles the intended repository. GitHub atomically rejects creation if an inaccessible repository owns the same name. An uncertain response never permits another create: `{"action":"reconcile","team_id":"TEAM_UUID"}` performs a read-only check under current organiser authority. Binding requires the exact numeric organisation, repository name, private visibility, original template ID and unique marker. Missing/inaccessible resources or missing provenance stay unresolved; a name alone never establishes ownership. Editing/removing the description before an uncertain operation is recovered therefore requires manual review rather than automatic adoption. The numeric repository identity and provenance cannot later be reassigned in the database.

Each team has its own durable operation, so failed teams do not roll back successful mappings. Disbanding retains the mapping; the native team owner deactivates the competition entry and the repository is unused/unscored. There is no automatic repository deletion, repository replacement, work transfer or code execution. Restoring database provenance and deployment keys is essential for reconciliation. The registry rejects deletion/truncation and a committed attempted flag cannot be reset. Populated rollback is blocked while registry or provisioning-operation history exists; an empty rollback preserves predecessor owners. Any manual recovery must preserve the original provenance and uncertain-create barrier.

The provider transport is tested against deterministic HTTP fixtures, and transactions/concurrent workers are tested with real PostgreSQL. Approved live sandbox configuration was verified for #91 on 27 October 2026; see the [live configuration evidence](verification/github-app/README.md). These transport tests do not establish real repository creation, invitations, webhooks or cutoff recovery; the separate full sandbox event remains pending under #98. GitHub's current [template creation contract](https://docs.github.com/en/rest/repos/repos#create-a-repository-using-a-template) documents the supported installation tokens, permissions and explicit private flag used by this implementation.
