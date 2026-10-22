# Event rules and contact guidance (#158)

Native resource pages can declare a rules, support or conduct purpose. Their typed
contact routes remain in the existing immutable content version, covered by the
same review, approval, release, withdrawal and audience checks. Challenges and
role briefs cannot acquire a guidance purpose. The guidance endpoint selects
only actually released native pages; private corrections do not replace the
current version in Today, preparation or resources.

Rules require a personal acknowledgement of each released version. The existing
append-only receipt records person, content, version and time. After current
resource access succeeds, the reader now projects the requesting person's
current receipt and latest earlier receipt. An earlier receipt never satisfies
a corrected version and another person's receipt is not returned.

Support and conduct pages have separate purposes. Contact routes accept HTTP(S)
or one mailto address, with no embedded credentials or prefilled mail text.
Starter templates contain unresolved placeholders and synthetic examples only.
They can be saved privately, but unresolved contacts cannot be approved. The
separate admin content workspace warns when a confirmed-competitor support or
conduct route is missing. These warnings do not alter lifecycle blockers,
preparation completion or competition admission.

The participant build has a minimal Today entry containing this guidance; this
is not completion of the broader #88 dashboard. Guidance is also reachable from
personal preparation and the resource library. Reading a page or acknowledging
it rechecks current access. Links are deliberate and no remote content is
fetched automatically. The app provides no report collection form or report
payload, does not feed reports to messaging/scoring/displays, and makes no
anonymity promise about an external contact route.

## Evidence

- `TestGuidanceCurrentReleasedRulesAndPersonalReceipts`: private corrections,
  exact current/prior personal receipt, cross-event access and withdrawal.
- `TestGuidanceContactsRequireRealConfigurationAndCurrentAudience`: placeholder
  draft versus approval, safe schemes, audience filtering, current membership,
  organiser-only nonblocking warnings and no lifecycle revision mutation.
- Existing real PostgreSQL content/acknowledgement/HTTP suites retain release,
  embargo, team/role and download guards. The guidance HTTP contract verifies
  draft non-disclosure, released metadata and `Cache-Control: no-store`.
- Twenty-four desktop/mobile content and guidance browser cases pass. They cover
  corrected rules, current personal acknowledgement, withdrawal, safe published
  routes, missing-contact warnings and unresolved starter contacts alongside
  existing content approval/conflict/scheduling journeys.
- Two additional desktop/mobile cases use an ordinary nonadmin identity in the
  separate participant build and reach the released route from Today and
  personal preparation without organiser navigation.
- Both frontend builds, web lint and Go lint pass. Generated SQL and Go/TypeScript
  contracts are reproduced from their source definitions.

Synthetic mobile Today/contact and corrected-rule captures were visually
inspected: text wraps within the viewport, routes/buttons remain readable and
no report input is rendered. No actual contact message or external report was
sent. This change needs no database migration.

The final rebase onto merged campaign/reveal work (`8253826`) retains all
campaign/MC and participant team-access navigation, every existing public test
entry and private print masks. Rebase-focused guidance+campaign cases pass20/20
in7.2s, and participant guidance+team-access+room-display cases pass34/34 in7.5s.
The guidance contact policy also rejects percent-encoded mailto control
characters after URI decoding, with PostgreSQL regression cases.
