import type { components } from './api/schema.gen'
export type MessageTemplate = components['schemas']['CommunicationTemplate']
export type MessageAudience = components['schemas']['CommunicationAudience']
export type CampaignDefinition = components['schemas']['CampaignDefinition']
export type CampaignDetail = components['schemas']['CampaignDetail']
export type CampaignPage = components['schemas']['CampaignPage']
export type ComposerPage = components['schemas']['CommunicationPage']
export type Channel = CampaignDefinition['channels'][number]
export const channelNames: Record<Channel, string> = { smtp: 'Email', teams: 'Teams broadcast', in_app: 'In-app inbox' }
export const purposes: MessageTemplate['purpose'][] = ['reminder', 'registration', 'offer', 'facilitator_selection', 'preparation', 'brief_release', 'challenge_release', 'feedback', 'results', 'postponement', 'help', 'clarification', 'rubric', 'outcome_review']
export const roles: MessageAudience['roles'] = ['attendee', 'organiser', 'mentor', 'judge', 'mc', 'logistics', 'photographer']
export const registrationStates: MessageAudience['registration_states'] = ['received', 'confirmed', 'waitlisted', 'offered', 'selected', 'backup']
export const blankTemplate = (): MessageTemplate => ({ id: crypto.randomUUID(), version: 0, name: '', purpose: 'reminder', format: 'text', subject: '', body: '', fact_id: null })
export const blankAudience = (): MessageAudience => ({ id: crypto.randomUUID(), version: 0, name: '', roles: ['attendee'], registration_states: [], teams: [], outstanding: '', fact_id: null })
export const blankCampaign = (source: ComposerPage): CampaignDefinition => ({ id: crypto.randomUUID(), name: '', template_id: source.templates[0]?.id ?? '', audience_id: source.audiences[0]?.id ?? '', channels: ['smtp'], include_calendar: false, timing: { kind: 'relative', wall_time: '', dst_choice: '', anchor: 'live_start', days: -1, minutes: 0, business_days: false } })
export function blockerText(code: string) {
  const messages: Record<string, string> = {
    ready: 'Ready', not_configured: 'Not configured', readiness_unavailable: 'Readiness could not be verified', address_unverified: 'Your account has no verified email address', broadcast_only: 'This shared broadcast cannot target only you',
    approver_authority_review_required: 'The campaign approver no longer has current organiser authority. Review approval before continuing.',
    preview_approver_authority_review_required: 'The retained preview approver no longer has current organiser authority. Generate and review a new preview.',
    provider_configuration_changed: 'A selected provider connection has changed. Review its readiness and explicitly reapprove the campaign.',
    approval_outdated: 'The saved approval no longer matches the current content, selector, dates or timing. Generate and review a new preview.', timing_invalid: 'Resolve the event-local time and daylight-saving choice.', message_or_audience_unavailable: 'Choose a current message and audience in this event.',
    teams_requires_common_audience: 'Teams broadcasts require the common confirmed-competitor audience without team, registration-state or outstanding-work filters.', teams_requires_broadcast_content: 'Teams cannot broadcast personal variables, private routing facts or personal-message purposes.',
    empty_audience: 'No current eligible people match this selector.', unresolved_placeholders: 'Some recipients have unresolved or embargoed values. Resolve them before approval.', unresolved_official_fact: 'The linked official publication or routing fact is not available.',
  }
  if (code.startsWith('provider_unavailable_')) return `${channelNames[code.slice('provider_unavailable_'.length) as Channel] ?? 'Selected channel'} is unavailable. Your selection is retained; complete provider setup before approval.`
  return messages[code] ?? code.replaceAll('_', ' ')
}
export const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b)
export function toggle<T>(values: T[], value: T) { return values.includes(value) ? values.filter(v => v !== value) : [...values, value] }
