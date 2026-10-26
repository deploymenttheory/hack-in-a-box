# Hack-in-a-Box documentation

Start with the [operator handbook](operator-handbook.md) to install, configure,
upgrade and troubleshoot an installation. Use the [event-day checklist](event-day-checklist.md)
to prepare and run an event. The [glossary](../GLOSSARY.md) and
[architecture decisions](adr/) define the product rules.

| Task | Documentation |
| --- | --- |
| Run the application and develop locally | [Repository README](../README.md) |
| Deploy independent participant/admin services | [Admin portal](admin-portal.md) |
| Monitor processes, storage and durable operations | [Operations](operations.md) |
| Back up, verify and restore a matching installation | [Encrypted recovery runbook](recovery-runbook.md) |
| Configure personal mail and recover uncertain sends | [SMTP](smtp-setup.md) |
| Configure an owned Teams Workflow | [Teams setup](teams-workflows-setup.md) |
| Link individual GitHub identities | [GitHub identity](github-identity.md) |
| Configure an organisation GitHub App and webhook | [GitHub App](github-app-setup.md) |
| Recover encrypted team credentials | [Credential recovery](team-credential-recovery.md) |
| Define and publish the shared scoring contract | [Rubric](rubric-contract.md) |
| Manage canonical answers and material changes | [Official clarifications](official-clarifications-design.md) |
| Plan objectives and distinguish estimated/evidenced value | [Objectives](objectives.md), [outcome follow-through](outcome-follow-through-design.md) |
| Publish and extend the independent interactive UI guide | [Style guide](design/styleguide.md), [contributor standards](../CONTRIBUTING.md) |

Verification records under [verification/](verification/) describe their own
source, fixture, execution and limitations. A historical screenshot or passing
component test does not establish a clean-machine deployment, live-provider
approval, capacity test or release signoff. The documentation walkthrough's
current status is recorded [separately](verification/operator-handbook/README.md).
