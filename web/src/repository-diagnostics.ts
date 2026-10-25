import type { components } from './api/schema.gen'
type S = components['schemas']
const explanations: Record<S['GitHubAppDiagnostics']['code'], string> = {
  not_configured: 'Configure an organisation-installed GitHub App and repository template.', installation_required: 'The App is configured; an organiser must complete the organisation installation.', healthy: 'The current App installation and template checks passed.', permissions_missing: 'The App installation lacks required permissions. Review each failed check.', installation_removed: 'The organisation installation was removed. Review and reconnect the App.', installation_suspended: 'The App installation is suspended. Resolve it in GitHub before provisioning.', organization_mismatch: 'The installation belongs to a different organisation. Review the configured organisation.', app_mismatch: 'The installation belongs to a different App. Review the configured App.', template_inaccessible: 'The App cannot read the configured template. Review its repository access.', template_invalid: 'The configured repository is not a usable template.', credentials_unavailable: 'App credentials cannot be read by this installation. Ask an installation administrator.', credentials_invalid: 'GitHub rejected the configured App credentials. Review the App ID and key.', provider_unavailable: 'GitHub is unavailable. Current checks are incomplete; do not assume provisioning failed.', permission_denied: 'GitHub refused this operation. Review App permissions and installation access.', rate_limited: 'GitHub is rate limiting requests. Wait before deliberately checking current readiness again.',
}
export function repositoryAppExplanation(code: S['GitHubAppDiagnostics']['code']) { return explanations[code] }

const repositoryMessages: Record<string, string> = {
  template_unavailable: 'The template could not be read. Review the App installation and template access.',
  template_invalid: 'The source is not a valid repository template. Review organisation setup.',
  provider_unavailable: 'GitHub could not be reached. Read current readiness before another review.',
  create_rejected: 'GitHub rejected repository creation. Review App permissions and repository setup.',
  create_uncertain: 'The creation outcome is unknown. Reconcile this team’s original repository; do not create a replacement.',
  identity_unproven: 'The repository’s managed identity could not be proved. Reconcile the original; do not adopt or replace it.',
  reconciliation_required: 'Current provider facts are incomplete. Reconcile only this team’s original repository.',
  identity_collision: 'The provider result does not match the retained managed repository identity. Do not adopt, replace or transfer it.',
}
export function repositoryDiagnostic(code: string) { return repositoryMessages[code] ?? (code ? code.replaceAll('_', ' ') : 'No diagnostic reported') }
