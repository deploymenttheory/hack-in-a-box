# SMTP and calendar contract

The SMTP adapter submits one personal message to one mailbox over authenticated,
verified TLS. It supports required STARTTLS or implicit TLS and never falls back
to an unencrypted authentication exchange. Header fields accept single safe
values; rendered HTML treats approved message text as literal text.

The durable provider result is transport acceptance only. A final positive DATA
reply means accepted; a later QUIT failure cannot undo acceptance. Explicit 4xx
rejections are retryable and 5xx rejections permanent. A lost connection/timeout
after sending DATA, without a definite rejection, is ambiguous and must not be
retried automatically. SMTP offers no generic trustworthy receipt lookup for
reconciling an uncertain send. These boundaries follow
[RFC 5321 sections 4.2.5 and 4.5.3.2](https://www.rfc-editor.org/rfc/rfc5321.html#section-4.2.5)
and the [Go SMTP client contract](https://pkg.go.dev/net/smtp).

Calendar attachments use METHOD:PUBLISH with one stable event UID. The caller
retains the authorised calendar sequence and snapshot for each immutable delivery;
retrying reuses them and an approved update advances the sequence. DTSTART and
DTEND use UTC instants, avoiding ambiguous floating dates or incomplete timezone
components. The event's IANA timezone appears in the date details. Text escapes
backslashes, commas, semicolons and newlines, and folding respects 75 UTF-8 octets
without splitting code points. See [RFC 5545](https://www.rfc-editor.org/rfc/rfc5545.html)
and [RFC 5546 PUBLISH](https://www.rfc-editor.org/rfc/rfc5546.html#section-3.2.1).

Mailboxes never identify an account or grant roles: identity remains issuer plus
subject. An authenticated OIDC email_verified=true claim may establish a private
claim-derived contact. Entra does not guarantee that claim, so an account-owned
neutral confirmation-code exchange is also required. An explicitly code-confirmed
mailbox is kept separate from claim-derived provenance and cannot be overwritten
by a later unverified hint. See [OIDC standard claims](https://openid.net/specs/openid-connect-core-1_0.html#StandardClaims)
and [Microsoft's ID-token claims reference](https://learn.microsoft.com/en-us/entra/identity-platform/id-token-claims-reference).

All transport verification uses a loopback SMTP fixture and synthetic example.test
mailboxes; no messages are sent to external recipients.
