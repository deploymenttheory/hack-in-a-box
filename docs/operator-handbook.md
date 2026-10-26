# Operator handbook

This handbook covers the Docker Compose installation, independent portals and
durable worker. Record the reviewed source revision and image digests for each
installation. Capacity planning targets up to 200 attendees per event and 1,000
active users across the installation; release load verification is tracked in
#123. These are acceptance targets, not a measured guarantee for an arbitrary host.

## Install on a fresh host

Use a checkout of a reviewed revision, Docker with Compose v2, and OpenSSL.
Keep this checkout, project name, environment file and volume inventory together.
The following commands create the default `hackbox` project; run them only for
the installation you intend to manage.

```sh
sh scripts/init-env.sh
docker compose build api admin
docker compose up -d --wait postgres
docker compose run --rm migrate
docker compose up -d --wait api admin worker
docker compose ps
curl --fail http://127.0.0.1:8080/readyz
curl --fail http://127.0.0.1:8081/readyz
```

The participant portal is at `http://127.0.0.1:8080`; administration is at
`http://127.0.0.1:8081`. The worker has no published HTTP port. Compose stores
PostgreSQL and protected files in persistent volumes. Recreating containers
retains them; `down -v` removes them and is not an upgrade or recovery procedure.
`init-env.sh` preserves an existing `.env`; it creates random database/metrics
credentials only for a new one. An existing environment must be reviewed when
new configuration keys are introduced.

An empty sign-in configuration permits the shell and health checks, not an
authenticated event. Configure a real approved OIDC client before the first
operator login. A synthetic-provider rehearsal must use its own isolated project,
identities and data; it must not grant a production authentication bypass.

## Origins, TLS and first administration

For production, set `HACKBOX_ENV=production` and use separate HTTPS origins for
the participant and admin services. Terminate TLS at your managed reverse proxy,
route each hostname directly to its corresponding loopback port, and retain the
private Compose database network. Do not route administration through the public
portal process. Shared PostgreSQL, files, identity provider and host remain shared
failure dependencies. See [admin deployment](admin-portal.md).

Set `HACKBOX_PUBLIC_ORIGIN` to the participant origin and `HACKBOX_ADMIN_ORIGIN`
to the admin origin in Compose. Keep `HACKBOX_WORKSPACE_ORIGIN` set to the canonical
participant origin for both APIs and the worker; mail/calendar links and approval
bindings use it. Origins contain scheme, hostname and optional port, without a
path. The admin container receives its own origin for session and CSRF checks.

Configure the exact OIDC issuer, client ID and optional confidential-client
secret. Register each portal's `/auth/callback`; separate admin client settings
are available in [.env.example](../.env.example). Identity uses issuer/subject,
not email. Set `HACKBOX_BOOTSTRAP_SUBJECT` to the first operator's stable subject,
log in, verify the platform grant, then remove the setting and recreate API/admin
containers. Bootstrap grants are audited. Platform administration alone does not
grant current event content, credentials or judging access.

Validate sign-in and logout independently on both HTTPS origins, current role
checks, certificate renewal, and a rejected cross-origin write. Production session
cookies require HTTPS. Readiness does not verify the external identity provider
or prove that a provider redirect is registered. Keep a controlled issuer/admin
recovery procedure with the identity-provider owner.

## Create a branded test event

Use synthetic people and an isolated rehearsal installation for this check.
After the first operator sign-in, complete these steps in the admin portal:

1. Choose **Create event**. Enter a synthetic event name, slug, description and
   venue. In **Attendance and dates**, choose a future live start/end and a
   submission cutoff within that live window. Choose final admission before team
   freeze, and team freeze before the computed T−7-day common release.
   Use the date preview to correct blockers before saving.
2. In **Organiser responsibility**, explicitly accept the event organiser role.
   Choose **Save draft event**, review the exact details, then **Confirm save
   draft event**. Verify the returned event name and draft state. Creating a
   draft neither approves the go decision nor releases the challenge.
3. In the event section selector choose **Branding**. Under **Colours**, choose
   contrast-valid colours; under **Images**, select a synthetic PNG/JPEG logo
   within the displayed limits. Inspect **Preview**, then **Save branding**.
   Wait for **Branding saved.**; a timeout does not confirm a save and must not
   be followed by an automatic retry.
4. Reload the event and verify its saved name, logo and colours. Open the same
   event in the participant portal using the current organiser account to verify
   its permitted reader. This checks that account's staff access; it does not
   establish confirmed-competitor eligibility.
5. Open the matching independent style guide, then sign out of one portal and
   confirm that the other retains its own session. Record the source revision,
   image digests, actual results and any corrections before removing the
   rehearsal's own disposable resources. Preserve every other installation.

## Runtime configuration and secret custody

Compose reads `.env` for interpolation; Go commands read exported process
variables. Only variables mapped in `compose.yaml` reach a container. For an
advanced Go setting absent from that mapping, add an explicit, reviewed Compose
override to the affected API/admin/worker services. Do not assume that adding an
arbitrary name to `.env` changes the process. Keep deployment overrides private
when they contain secrets. Do not publish expanded Compose configuration.

| Settings | Operational requirement |
| --- | --- |
| `HACKBOX_DB_PASSWORD` | Private database credential; changing it in `.env` does not rotate an existing PostgreSQL role automatically. Plan database and application changes together. |
| `HACKBOX_METRICS_TOKEN` | At least 32 characters; keep scraping access separate from public traffic. |
| OIDC settings | Same reviewed issuer/account policy, correct per-portal callbacks; rotate with the identity-provider owner. |
| `HACKBOX_CREDENTIAL_KEYRING` | Retain historical key IDs/material while ciphertext or backups need them; use the documented active-key rotation procedure. |
| `HACKBOX_GITHUB_TOKEN_KEY` | Separate participant OAuth token-encryption key; retain matching material for the selected backup. |
| SMTP settings | Same transport and canonical workspace origin for APIs/worker; verified TLS and authenticated relay. |
| Body/file/quota limits | Account for base64 upload overhead; do not set a body limit below the allowed encoded file plus JSON metadata. |
| `VITE_STYLEGUIDE_URL` | Public build-time URL for both portals; rebuilding is required after a change. Never place a secret in a `VITE_` value. |

Keep `.env`, issuer/client secrets, provider keys and keyring material outside
Git, images, screenshots, CI artifacts and database/file recovery bundles. The
application's database can contain encrypted provider credentials, so database
backups remain private even when ciphertext is present. Archive encryption uses
a separate recovery identity. Test retrieval of that identity and historical
deployment keys without relying on the source host.

See [credential recovery](team-credential-recovery.md) and the
[encrypted backup/runbook](recovery-runbook.md) before rotating or retiring keys.
Do not regenerate keys to make a decryption error disappear.

## Enable integrations deliberately

| Integration | Prerequisites and ownership |
| --- | --- |
| Personal mail | [SMTP setup](smtp-setup.md): approved sender/domain, authenticated TLS relay, confirmed contact and a current exact campaign approval. Configuration checks do not send a message. |
| Teams broadcasts | [Teams Workflow setup](teams-workflows-setup.md): an approved tenant/channel endpoint and named owner/replacement owner. Treat its URL as a secret. A mocked endpoint proves transport behavior, not receipt in a real channel. |
| GitHub identity | [Participant OAuth](github-identity.md): a distinct OAuth application and each portal's linking callback. Organisation SSO does not prove a GitHub account or accepted repository invitation. |
| Managed repositories | [GitHub App](github-app-setup.md): approved organisation installation, private key, template visibility and current repository/operation permissions; reviewed repository/access plans. |
| Signed repository activity | Same App's configured webhook URL/secret and required subscriptions. External invitation acceptance and cutoff evidence must be reconciled through their native workflows. |

Use approved sandbox credentials for provider rehearsals. Until the real
organisation/App installation and Teams tenant endpoint exist, mark those live
checks **not run**. An HTTP 2xx from a synthetic provider or an invitation sent is
not a live tenant receipt or an accepted collaborator grant. Keep live evidence
out of public logs when it identifies people or private repositories.

## Portal navigation and the published UI guide

The participant portal provides current event participation and assigned work;
the independent admin portal provides event creation, configuration and organising
workspaces. Choose the current event, then its labelled section; narrow screens
use the same section selector. Current authority determines which destinations
and actions appear. A hidden or visible destination does not replace server-side
authorization. Ideas & projects is a separate top-level, current-permission reader.

Publish the guide independently:

```sh
docker compose -f compose.styleguide.yaml --profile styleguide up -d --build --wait
```

It runs at `http://127.0.0.1:8082` in its own project, without app/database/worker
configuration. In production, publish the artifact corresponding to the deployed
component revision on the approved docs origin and build both portals with that
`VITE_STYLEGUIDE_URL`. Retain its entries, fingerprinted assets, license and host
security headers together. Updating runtime environment variables cannot rewrite
a compiled footer URL. Follow [guide publication and contributions](design/styleguide.md):
change shared production tokens/components, interactive examples and usage rules
together, then run the guide checks. Promote matching application and guide
versions; do not overwrite the old guide before an upgrade is validated.

## Upgrade and rollback

1. Record current source/image digests, configuration, application/River schema,
   volumes, provider configuration versions and outstanding operation states.
   Choose a maintenance window; prevent new user writes and notify organisers.
2. Stop this installation's API, admin API and worker plus any independent writers.
   Keep the previous images. Back up PostgreSQL and referenced files together
   using the matching recovery image and verify the encrypted bundle with the
   separately held identity. Confirm a fresh-target restore is usable before
   relying on a new backup. Follow the exact [recovery runbook](recovery-runbook.md).
3. Check out the reviewed new revision, retain the deployment's private environment
   and volume mapping, and build/tag its public/admin/recovery images. Review
   changed `.env.example` settings and the matching independent guide revision.
4. Run the explicit `docker compose run --rm migrate` with the intended new public
   image. Migrations serialize application/River changes. A failed migration is
   not permission to start old or incompatible binaries; inspect status and the
   matching migration/recovery procedure before any further write.
5. Start the new APIs and worker only after successful schema checks and any
   recovery hold has been reviewed through the native recovery procedure. Verify
   readiness, both normal logins, a synthetic scoped read/write, protected file
   access, separate logout, guide links and durable operation status.
6. Keep the old image/backup/key inventory until the upgrade is accepted. A rollback
   across schema changes restores the matching database/files/key set into a
   fresh target and deploys compatible binaries; changing the image tag alone
   does not downgrade the schema. Review pending external effects before worker
   resume. Do not approve or resend operations to clear an alert.

## Diagnose without replaying unknown effects

| Symptom | First checks and approved next action |
| --- | --- |
| Process unhealthy | Check only the intended project's container state and bounded logs; correlate safe request IDs. Readiness checks PostgreSQL/schema/files; liveness alone does not. |
| One portal down | Open the other directly; check its own process/proxy/OIDC route. Shared infrastructure may still affect both. |
| Sign-in loop or 401 | Check exact origin/issuer/callback, expired/revoked session and current membership. Do not relax TLS or link identities by email. |
| 403 or private details disappear | Recheck current person/event/team/resource scope. A former role or historical link does not restore authority. |
| 409 on a reviewed action | Refresh current facts and deliberately review the new version. Preserve the old attempt identity; do not treat a read as its acknowledgment. |
| Unknown send, upload or write outcome | Inspect original attempt/status through the owning workflow. A timeout is not proof of no effect; do not repeat under a new identity. |
| Pending delivery or automation pause | Check event pause, approval/source binding, audience, retention, worker and provider status. Due time alone is not approval. |
| Missing/corrupt protected file | Preserve the metadata and restore the matching verified object from the consistent backup; do not fabricate an empty replacement. |
| Decryption failure | Check exact historical key IDs/material and provider credential version; follow the recovery runbook. |

Use [operations](operations.md) for the actual metrics and durable delivery
contract. Aggregate pending-job lag can include source facts or unavailable
handler kinds; inspect the operation's kind and current eligibility before
concluding that a runnable delivery missed its deadline. Readiness does not
prove a live provider is healthy. Record the approved recovery action, original
operation identity, actor, reason and observed result; preserve audit history.

The [event-day checklist](event-day-checklist.md) covers competition and result
release decisions. The [walkthrough record](verification/operator-handbook/README.md)
keeps documentation checks separate from deployment/release evidence.
