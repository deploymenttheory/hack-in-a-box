# Archived score-bearing report regression

Correction based on frozen integrated issue113 commit `6dd4572a6c9183b1421dca4ac677bdc82b049758`, developed in the isolated `work-results113-archive` branch.

The native rubric read-assessment stage gate rejected `archived` before the report owner could validate its deliberately included confirmed scores. The report owner converted that unavailable source into a conflict, hiding the released report even though its audience and evidence had not changed. The correction admits archived events only to the existing read-assessment path. Its published rubric/run, frozen submission, current roles, assignments, conflicts and qualification checks remain in place. Assessment start and report/judging write gates are unchanged.

## Native RED

Before changing production source:

```sh
HACKBOX_INTEGRATION=1 go test ./internal/app -run '^TestTeamReportsArchivedReadNeverGrantsWrites$' -count=1 -timeout=10m
```

Terminal FAIL in **9.721s**. Both `all_teams` and `mentor_qualification` failed at the released report read immediately after the actual native event transition from completed to archived, returning a stale resource revision. The same report was readable immediately before that transition.

## Native GREEN

```sh
HACKBOX_INTEGRATION=1 go test ./internal/app -run '^TestTeamReports(ArchivedReadNeverGrantsWrites|CompletedReadNeverGrantsScoringWrites)$' -count=1 -race -timeout=10m
```

Terminal PASS in **38.421s**, four serial native PostgreSQL cases: completed and archived, each in both competition modes. Actual result confirmation, explicit score selection, draft and separate report release precede the archive transition. Assertions cover:

- Exact equality of the authorised recipient's released report before and after archive, including deliberately selected own-team rational scores.
- Exclusion of private mentor/judge prose, rank, awards, vote/evidence records and captured audience from the recipient DTO.
- Archived report save/release and judging-card writes remain forbidden.
- Current recipient withdrawal denies access; current judge-role, assignment and conflict changes invalidate the score source.
- Withdrawn frozen evidence and unavailable rubric publication invalidate current scores in private reads, history and recipient reads.
- Source/authority projections do not rewrite retained report documents.

An intermediate expanded run failed because two test setups did not respect their actual owners: an open final entry automatically freezes after cutoff, and a superseded rubric run cannot be restored. The final test uses withdrawn final evidence and removal/restoration of the existing rubric publication pointer. No production authority or persistence guard was relaxed to accommodate those setups.

Configured affected Go lint (`rubric`, `teamreports`, `results`, `judging`) passed with **0 issues**; configured formatting and `git diff --check` passed. No frontend, category voting policy, API/generated contract or migration changes are included. This focused correction does not repeat or supersede the integrated native browser matrix.
