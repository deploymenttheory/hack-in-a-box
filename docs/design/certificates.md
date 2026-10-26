# Optional verified recognition (#164, M5)

Certificates recognise recorded attendance or explicitly published awards. They
are not qualifications, payments or publicly enumerable credentials. The separate
#165 interface is not part of this backend deliverable. No notifications are sent.

An organiser explicitly reviews versioned plain-text recognition/issuer templates
and decorative branding. Attendance is disabled until a policy specifies event-local
live dates and a minimum number of recorded-present days. Registration alone never
qualifies. The current attendance owner records confirmed competitor arrivals; these
certificates use that same current confirmed-recipient boundary. Award eligibility
comes only from the latest explicit award release and its published frozen team.
Private result preparation, confirmation or reopened judging never supplies a
publication. A later explicit award release requires a newly reviewed source.

A preview binds current eligibility and approved printable fields. Explicit issuance
must use that binding. Corrections keep the certificate UUID and append provenance;
revocation is terminal. Staff-only immutable history carries the exact original
request/action/fingerprint receipt as well as actor and reason. Recipient reads
never receive private issuance reasons or staff history. Current source changes block printing until an eligible,
explicit correction. Current organiser or own-recipient access is rechecked on every
read; UUID possession grants nothing. The printable allowlist excludes scores,
assessment prose, contact details and sensitive registration answers.

Settings/issue/correct commands require current event-write authority; explicit
revocation remains available to current organisers to stop exposure even after
ordinary writes close. Nothing changes frozen competition evidence.

Implementation and real PostgreSQL/HTTP verification remain in progress; this
checkpoint does not satisfy #164 or an M4/release gate.
