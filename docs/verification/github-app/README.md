# Approved GitHub App configuration verification (#91)

On 27 October 2026 the approved `deploymenttheory` sandbox App completed the
normal organiser configuration → reviewed installation handoff → GitHub owner
approval → session-bound callback → current readiness journey. The organisation
owner completed GitHub's installation approval in the same Chrome session. No
SQL mutation, copied session cookie or test authentication endpoint was used.

The running API/admin/worker source was
`ba5a7c2b66b0179b9de52a5188517780a92ad6f8` (merged PR #304). The supporting
native tests were rerun on `1d283469`, whose only subsequent change is CI action
pinning. The local origins were public `http://127.0.0.1:8080`, administrator
`http://127.0.0.1:8081`, independent guide `http://127.0.0.1:8082`.

## Observed live result

[Safe configuration proof](configuration-proof.json) records the actual current
HTTP 200 response obtained through a separate normal OIDC sign-in after the
browser callback, followed by the visible Check current App readiness action.
It contains public App/organisation/template identifiers and safe diagnostics;
no signing key, token, webhook secret, state, cookie or CSRF value is included.

| Item | Observed |
| --- | --- |
| App | `deploymenttheory-hackbox-sandbox`, ID `5486012` |
| Organisation | `deploymenttheory`, ID `103241062` |
| Private selected template | `hackbox-sandbox-template`, ID `1434900904` |
| Recorded installation | `175693791` |
| Event | `2b196e07-e42c-420e-ab38-d7faf8f87ebd`, configuration revision 4 |
| Current provider health | `healthy` |
| Template metadata read | Allowed |
| Template-based private repository creation permissions | Allowed |
| Repository invitation-management permissions | Allowed |
| Repository activity-read permissions | Allowed |
| Webhook | Not configured; separate #98 recovery work remains |

Before the completed callback, the same native reader returned configuration
revision 3, installation ID 0 and `installation_required`. The successful callback
therefore changed the recorded integration; an external GitHub page alone was
not treated as proof of connection.

## Acceptance mapping

- Organiser/current-event authority, session/state binding, wrong-organisation
  refusal, replay/stale/concurrent callback handling and read-only health:
  `TestAppConfigurationStateSessionAuthorityAndReplay`,
  `TestAppCallbackConcurrentSingleUseAndStaleConfiguration`,
  `TestAppHealthRemainsReadableWhenEventWritesAreDisabled`, and
  `TestGitHubAppHTTPStrictPrivacyCSRFAndSessionBoundCallback`.
- Operation/repository-limited token requests and no installation-wide fallback:
  `TestProviderVerifiesAppOrganizationTemplateAndLeastPrivilegeTokens` and
  `TestOperationTokensNeverDefaultToInstallationWideAccess`. The actual live
  template health check also succeeded through the production provider.
- Missing permissions, removed installation, inaccessible template and credential
  fault/recovery diagnostics: deterministic provider scenarios above and
  `TestAppKeyFaultRotationRecoveryAndDisconnect` with real PostgreSQL.
- Endpoint-specific read/create/invitation permissions: all four actual current
  permission diagnostics passed; deterministic transports verify the requested
  scopes rather than treating these permission diagnostics as completed writes.

The fixture tests establish failure handling and confidentiality; they do not
pretend those failures were induced in the real organisation. This live exercise
establishes approved configuration acceptance only. It created no team repository,
sent no collaborator invitation and exercised no actual webhook or cutoff.
The full synthetic repository/recovery event remains with #98 and its
[operator execution record](../github-recovery/SANDBOX-RECORD.md).
