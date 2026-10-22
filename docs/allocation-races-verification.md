# Allocation invariants and publication races

Issue #68 exercises the registration, team, mentor, access and submission services
against real PostgreSQL using synthetic confirmed registrations. The tests start
at application, publish two teams and record the go decision through application
services before crossing domain boundaries.

`internal/registration/allocation_races_test.go` covers:

- Two organisers publish the same reviewed revision concurrently. Exactly one
  commits and emits one publication intent. Unpublished moves preserve old access;
  publication removes it and grants the new team access. Assigned people still
  have confirmed places, and the reviewed 3/5 allocation remains valid despite
  falling outside the preferred four-to-six range.
- Freeze races manual moves and queued publication. One reviewed revision wins;
  stale retries conflict and fresh post-freeze moves/publications are rejected
  without changing competitor membership.
- Freeze races actual registration withdrawal. Withdrawal always succeeds. If it
  wins first, the fresh under-three team blocks freeze pending remediation; if
  freeze wins first, the smaller team continues. Neither outcome moves survivors.
- Under-three remediation moves the remaining two before freeze and retires the
  emptied entry. Its historical repository reference remains attached to its
  private preparation artifact, transfers nowhere, becomes inaccessible to former
  members and contributes no active scored revision. The remaining frozen team
  can later continue with two competitors.
- Mentor publication races a declared reporting conflict after competitor freeze.
  Exactly one reviewed command commits. Recording the conflict revokes access;
  acknowledging coverage gaps cannot override the hard conflict.
- Mentor replacement publication races actual registration withdrawal after
  freeze. Publication either commits first or requires a fresh roster review.
  The old mentor and withdrawn competitor lose access, the replacement gains
  access, and every remaining competitor stays on their frozen team.
- Generating allocation suggestions preserves the participant snapshot and writes
  no operation for notification or repository provisioning. Eight competitors
  produce two preferred teams of four.

`web/e2e/allocation-lifecycle.spec.ts` follows the organiser through replacement
in the independent admin portal, a withdrawal invalidating the reviewed
publication, fresh review/publication and inspection of the depleted frozen team.
Desktop and mobile both verify no competitor mutation request and no reassignment
or freeze controls. The browser uses deterministic API fixtures; the transaction
and race guarantees above use PostgreSQL, not these browser mocks.

Verification during implementation:

- Related real-PostgreSQL race suites: registration 31.225s, teams 14.591s,
  mentors 11.762s, access 3.547s and submissions 7.317s, all passing.
- Additional actual withdrawal/mentor replacement race: 3.716s, passing.
- Combined browser scenario: desktop and mobile, two tests passing in 2.0s.
- Frontend build/lint and Go lint pass. The implementation PR records CI results
  for the final head.

GitHub repository provisioning remains a separate backlog deliverable. These
checks prove that unpublished suggestions emit no delivery intent and retired
repository references are unused/unscored; they do not claim a live provider
repository has been provisioned or its remote permissions changed.
