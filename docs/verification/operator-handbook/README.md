# Operator documentation verification (#125)

The documentation now has an operator handbook, event-day checklist and real
index instead of the inherited project template. It describes current Compose
service/origin/configuration boundaries and links the owned integration/recovery
procedures. README statements that all integrations are future work are removed;
the recovery guide no longer recommends a stale hard-coded schema.

This is an in-progress deliverable. No clean-machine branded-event walkthrough,
full #120 lifecycle or final release signoff is claimed by writing these pages.

| Required proof | Current status |
| --- | --- |
| Local links and documented commands/settings checked against owned source | Pending independent check |
| Secret examples contain placeholders only | Pending independent check |
| Fresh isolated Docker installation, normal sign-in, branded test event, matching guide | Not run for this documentation candidate |
| Correct any steps discovered by the actual walkthrough | Awaiting that execution |
| ADR-0001–0005, retention/archive exceptions, integration/manual recovery and optional M5 accuracy | Pending independent Spec review |
| Independent Standards review and final candidate CI | Pending |

The fresh walkthrough must use a separate project, volumes, ports and synthetic
identity/provider fixtures, preserve existing local installations, and record its
source/image digests, actual commands/outcomes, portal logins, event branding and
cleanup. A Compose health-only smoke does not satisfy the branded-event criterion.
A fixture using seeded sessions is not normal OIDC evidence. Live GitHub/Teams
checks require their separately approved external prerequisites.

The integrated walkthrough depends on #120's coherent synthetic lifecycle and
the accepted current UI/runtime. Until its proof exists, keep #125 and its PR open.
