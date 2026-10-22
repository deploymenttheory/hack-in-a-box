# SMTP setup and recovery

Configure the API and worker with the same SMTP settings. Leave all SMTP settings
empty to disable email honestly; configured campaigns cannot pass readiness until
a transport is present. Runtime construction and readiness checks send no mail.

| Variable | Value |
| --- | --- |
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
errors are bounded and do not echo credential values. A transport, public origin
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
Production campaign scheduling remains a separate worker integration.

The receipt means **relay accepted**, never delivered to an inbox or read. A final
positive DATA reply remains accepted even if QUIT subsequently fails. A definite
temporary rejection may retry with the same immutable message ID, body, calendar
UID and sequence. A connection loss after DATA with no definite reply is ambiguous;
review relay evidence before requesting another test. Do not reset prepared or
ambiguous snapshots to pending: they may already have caused an external effect.
A receipt-storage failure is also uncertain. Restoring a backup does not prove a
message was unsent; keep workers paused while reconciling unresolved effects.

Local automated checks use only synthetic addresses and loopback SMTP fixtures.
See [the protocol contract](smtp-contract-notes.md) for transport/calendar details.
