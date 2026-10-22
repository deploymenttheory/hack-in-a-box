# SMTP setup and recovery

Configure the API and worker with the same SMTP settings. Leave all SMTP settings
empty to disable email honestly; configured campaigns cannot pass readiness until
a transport is present. Runtime construction and readiness checks send no mail.

| Variable | Value |
| --- | --- |
| `HACKBOX_WORKSPACE_ORIGIN` | Canonical attendee origin shared by public/admin APIs and worker; HTTPS in production |
| `HACKBOX_SMTP_HOST` | Relay DNS host with a trusted TLS certificate |
| `HACKBOX_SMTP_PORT` | Relay port; default 587 |
| `HACKBOX_SMTP_TLS_MODE` | `starttls` (default, mandatory upgrade) or `tls` |
| `HACKBOX_SMTP_USERNAME` | Relay authentication username |
| `HACKBOX_SMTP_PASSWORD` | Relay password supplied through deployment secrets |
| `HACKBOX_SMTP_FROM` | One plain sender mailbox, without a display name |
| `HACKBOX_SMTP_MESSAGE_DOMAIN` | Controlled domain for stable message/calendar IDs |
| `HACKBOX_SMTP_TIMEOUT` | Bounded Go duration, default `10s`, maximum `20s` |

Use a relay authorised to send from the configured sender. Configure that domain's
SPF/DKIM/DMARC with the relay operator. The application requires authenticated TLS
with certificate verification; there is no insecure certificate or plaintext
fallback. Keep credentials outside source control and database backups. Configuration
errors are bounded and do not echo credential values. A transport, canonical workspace origin
or branding change invalidates approval binding; review and approve again.

Each signed-in person can confirm their own mailbox on Account using a neutral,
eight-digit email code. This supports Entra without trusting an unverified email
hint. A generic OIDC verified-email claim can supply a claim-derived contact;
a manually confirmed address remains independently confirmed across later login
hints. Mailboxes never grant membership, roles or account identity. The application
shows masked contacts only, and campaign APIs never accept a replacement recipient.

Confirmation codes expire after ten minutes, permit five failed guesses, and are
bound to the current person's session revision. Requests have a sixty-second
cooldown and person/destination hourly limits. The account UI reports relay
acceptance separately from confirmation. An uncertain request is not automatically
resent; if its message arrives, the original code may still prove mailbox control.
Expired verification evidence is removed after 24 hours by worker maintenance.
Protect the private contact and message-snapshot tables in backups and operator
access; rendered mail and recipient addresses are intentionally private data.

For a manual smoke test, configure a controlled test relay and sign in as the
intended organiser, confirm that person's mailbox, create/review a campaign, then
choose **Send test to me**. This is an explicit single-recipient test; it cannot
redirect to an audience or a shared channel. A calendar attachment must be selected
explicitly. Tests use a separate calendar UID from the live event, so receiving a
test cannot replace a live calendar entry. Queued tests expire after fifteen
minutes and recheck current role, mailbox, source, campaign and provider settings.
Production campaign scheduling remains a separate worker integration. That
integration calls the SMTP queue inside its own transaction; the queue resolves
the current approved audience once, stores individual private immutable intents,
and enqueues durable jobs without network effects. Occurrence-scoped sends also
require the automation policy to confirm the retained occurrence/source binding
and current recipient scope again before delivery. Missing scope policy fails
closed. Repeating the same approved occurrence deduplicates; a distinct approved
occurrence retains its own personal message identity.

The receipt means **relay accepted**, never delivered to an inbox or read. A final
positive DATA reply remains accepted even if QUIT subsequently fails. A definite
temporary rejection may retry with the same immutable message ID, body, calendar
UID and sequence. A connection loss after DATA with no definite reply is ambiguous;
review relay evidence before requesting another test. Do not reset prepared or
ambiguous snapshots to pending: they may already have caused an external effect.
Each network attempt retains an append-only prepared record linked to its
operation, source and attempt number. Its bounded receipt (outcome, safe code and
time) appends separately and commits atomically with the current status projection.
Retries preserve prior attempt receipts; unresolved prepared attempts have no
invented result. Operator reconciliation reads `smtp_submission_attempts` joined
with `smtp_submission_receipts`, rather than reconstructing overwritten snapshot
status. These history tables contain no addresses, content or raw relay replies.
A receipt-storage failure is also uncertain. Restoring a backup does not prove a
message was unsent; keep workers paused while reconciling unresolved effects.

Local automated checks use only synthetic addresses and loopback SMTP fixtures.
See [the protocol contract](smtp-contract-notes.md) for transport/calendar details.

An approved recipient without a verified mailbox produces an immutable
`address_unverified` omission record, with no address, body, send or invented
receipt. Repeating the same queue deduplicates that record. Confirming a mailbox
later does not silently send the old intent: an explicit reviewed intent or
occurrence review is required. Reports distinguish newly queued intents,
unavailable people and retained preexisting statuses. All-unavailable reports
must commit their omission evidence; they must not be called delivered.

An explicit same-occurrence omission review must first run the SMTP review guard
in the review transaction and wait for prior pending/running/prepared effects to
settle. A new review generation may queue only people previously omitted; existing
accepted, permanent or ambiguous intents retain their identity and status. A
changed source approval/version requires a distinct occurrence. Accepted always
means relay acceptance, and an ambiguous result remains unknown.
