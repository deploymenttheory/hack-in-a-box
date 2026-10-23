# Objective and success-measure definitions

Objectives are optional event planning configuration. Their owner is an optional
current organiser. Measures define descriptions, decimal targets, units,
measurement methods, evidence-source instructions and review horizons. Missing
baselines remain null; an explicit decimal zero remains zero. Baseline estimates
and self-reports have distinct labels. Configuration does not accept a claim of
a demonstrated achieved result: a future outcome owner must retain its native
evidence and exact objective-version/measure reference. No employee activity
ranking, monetary return calculation or attendance/publication gate is added.

Definitions and retirements append immutable versions under the current event
lock. The exact retained version identifies the unit and instructions used for an
observation even after a later edit. Current organiser authority applies to
configuration/list/history/owner-selection on every request, including archives.
Write-freeze denies new versions while authorised retained reads remain possible.
Advisory missing-input hints do not alter event lifecycle gates.

Template snapshots contain active configuration by value. Event cloning remaps
objective/measure identities, clears person ownership, and atomically creates
fresh version1 definitions. Approvals, disclosures, realised outcomes, private
evidence and follow-up/version history are not represented in the copy contract.
Generic event-definition editing cannot bypass native objective version history.

Native content documents may deliberately select exact immutable objective and
measure references for disclosure. The existing content review/approval/release
and current audience remain authoritative. Public projection has no privileged
preview fallback and omits private owner identities and planning baseline
self-reports. Only references in the current released readable content version
resolve. Withdrawal and audience/person revocation remove future access. Explicit
per-objective theme sensitivity blocks disclosure before the actual common
challenge publication; an initial challenge may include the same reviewed
references only inside its existing guarded release transaction. Other operational
objectives are not assumed to reveal a theme.

Stable native references are available to reporting owners #117/#119. This
backend does not implement achieved-result writes or the reporting dashboard.
The objective editor is owned by #157.

## Current verification checkpoint

Native PostgreSQL/race tests cover retained definition versions, two-event
isolation, current organiser revocation, concurrent revision conflict, immutable
retirement history, real native content disclosure/audience/embargo/withdrawal,
and actual event clone independence. Strict authenticated HTTP tests cover
private configuration/list/history/owner selection, no-store, stale If-Match,
explicit zero versus unknown, rejected fabricated achieved fields and inactive
session denial. Pure template/model tests cover ID remapping, owner clearing,
independent values and bounded exact decimal parsing.

The schema/queries currently remain provisional outside Goose, compiled through
an isolated temporary sqlc configuration. Concrete normal migration numbering,
ordered preservation/guarded rollback, canonical generated-query integration and
final full integration gates are required before this work becomes a PR.
