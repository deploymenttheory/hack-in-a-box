# Objective and success-measure editing (#157)

The separate admin portal's Objectives section uses the current native #156
planning API. It is optional configuration: missing owners, baselines, targets or
measurement instructions remain advisory gaps. A target is a goal, unknown is
not zero, and baseline estimates/self-reports are labelled. These forms record
no demonstrated results and add no lifecycle gate.

Private inputs stay in memory. Native current event, owner, list and exact
objective reads refresh every 15 seconds. Transport failures retain safe unsent
inputs while disabling writes; current native denial clears the editor and its
scoped private cache. Leaving the editor, changing event/account/session or
native denial fences all pending continuations. Cancellable reads consume abort
signals, and every subsequent request/cache update checks its original lifetime.
A sent write can still be accepted by the server: departure does not roll it back
or replay it, and a late response cannot revive the departed editor/cache.

Saving preflights current authority and the exact version. A changed definition
requires an explicit comparison/new-base choice without replacing local inputs.
Validation rejection remains distinct from an unknown transport outcome. An
unconfirmed write is reconciled against its exact next retained version, actor,
reason and complete definition. Only explicit unchanged-body/base retry is
available when current native state supports it. A retained receipt never becomes
the latest editing base merely because a newer version exists. Navigation warns
before discarding unsent work.

Templates author the same by-value fields without person-owner selection. Their
changed review/save/publish flows use the same lifetime fence and fresh revision
checks. Native cloning clears owners/remaps IDs and makes independent event
configuration; later event edits do not change the template.

Native content authoring optionally selects exact objective and measure versions
for its existing review/release workflow. A retained reference is never silently
retargeted to the latest planning definition. Authoring previews clear on current
planning denial. The attendee resource reader uses only the exact public
projection through currently readable released content. It never calls private
planning endpoints; owner identities and baseline self-reports are absent. Native
content approval/publication and common-release secrecy rules remain authoritative.

## Verification

Synthetic browser fixtures exercise real forms/routes and typed native HTTP
contracts. The composed admin run passes 66 desktop/mobile cases covering the new
planning, versioned disclosure and template reuse paths alongside existing
content/setup flows. The participant build passes 6 disclosure/withdrawal cases.
Keyboard/mobile checks include exact zero/unknown, optional owner selection,
changed units/definitions, stale-base review, validation, unsent navigation,
accepted/uncommitted response loss, active denial and in-flight event/session
cleanup. Native backend/source/clone/embargo enforcement is independently covered
by the merged #156 PostgreSQL and authenticated HTTP suites.

Fifteen actual mounted-component/cache tests deliberately return native replies
after cancellation to verify preflight/accepted-write/exact-outcome cleanup across
event/account/denial, plus template review/save cleanup. Substituting the pre-fix
objective editor makes its nine regression cases fail. Both frontend builds and
web lint pass. Mobile planning, authoring and participant captures were visually
inspected: private script-like text stays inert, labels/history wrap, and no
horizontal overflow occurs. Final exact-head CI and source review remain required.
