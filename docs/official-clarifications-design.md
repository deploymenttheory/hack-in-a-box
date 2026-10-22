# Private questions and official clarifications

Issue #161 owns the native question, review and publication contract. Issue #162
owns its complete interaction surface. A minimal canonical published-answer reader
must exist before an inbox destination is enabled.

## Authority and source

Every operation locks the event and rechecks the current active person. Questions
require confirmed competition status and current membership of the published team.
Staff access requires a current organiser role; assignment as a mentor or judge
does not disclose private questions. Archive preserves authorised historical reads
but disables every write. All challenge-specific surfaces require actual common
challenge publication, including staff triage and guessed identifiers.

The server captures the exact released challenge version and its exact published
rubric reference. Clients cannot supply a draft or substitute the latest rubric.
Question text and attachments remain private even when an answer is published.
Answers use a separate public projection without question identifiers, question
text, attachment references or team identity.

## Records and transitions

Private questions retain their original author, team, source reference and body.
Staff triage changes have separate revisions and audit records. Official answer
versions are immutable. Authoring, approving and publishing are distinct commands
bound to an exact answer version; every command records actor, time and result.
Approval and publication recheck current staff authority and source validity under
the event lock. Repeated publication returns the retained publication without a
second notification.

An answer is either an interpretation of its exact published source or an
announcement of a material change already completed through the owning challenge
or rubric workflow. Material-change publication requires durable owning release
and, for rubric changes, the reviewed impact and notice records. No answer command
accepts scoring overrides, rubric weights or an approval-bypass flag. Formal
assessment consumers continue to use immutable rubric run versions.

Corrections create another immutable answer version and explicit approval and
publication records. Public history retains the earlier published revisions and
their applicable sources. Draft and merely approved revisions never enter public
search, copied-link readers or inbox previews.

## Attachments and notifications

Question attachment references must belong to the same event and team. They never
become answer attachments implicitly. A question-scoped byte reader must first
authorize the question and association, then verify stored size and checksum.
Staff byte access additionally respects the existing live-start gate for team
working files; generic file policy is not widened.

Publication derives the current confirmed-recipient set in the same transaction
and records one native inbox source per answer revision and recipient. The source
contains only answer ID and revision, with fixed safe notification text. The
inbox owner rechecks current admission, actual publication and the requested
published revision on list, open and read. It returns only the canonical answer
destination. Shared inbox destination enums, owner maps and policy-resource checks
must preserve the independent help journey from #159.

## Verification before publication

Use two teams and two events with a controlled clock. Exercise prepublication
denial, copied links, private attachments, public DTO/search isolation, withdrawal,
staff revocation, archive, concurrent approve/publish/correct and duplicate inbox
delivery. Assert actual database state and HTTP no-store responses, not only
component visibility. Verify material-change rejection until the owning approved
release and impact/notice workflow completes. The minimal reader must not persist
private question bodies or mix them into common answer caches.

This is a design checkpoint, not a claim that the feature or migration is delivered.
Migration numbering follows the actual merged schema when the implementation is
ready; no provisional number blocks another lane.
