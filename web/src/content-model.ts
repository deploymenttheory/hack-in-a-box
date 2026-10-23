import type { components } from './api/schema.gen'

export type ContentPage = components['schemas']['ContentPage']
export type ContentDocument = components['schemas']['ContentDocument']
export type Attachment = components['schemas']['ContentAttachment']
export type ContentInput = components['schemas']['ContentInput']
export type ContentRole = ContentDocument['audience']['roles'][number]
export type Team = components['schemas']['HackTeam']
export type ContentTemplate = { id: string; name: string; input: ContentInput }
export const contentRoles: ContentRole[] = ['organiser', 'mentor', 'judge', 'mc', 'logistics', 'photographer', 'early_brief_access']
export const roleLabel = (role: string) => role === 'mc' ? 'MC' : role === 'early_brief_access' ? 'Early brief access' : role[0].toUpperCase() + role.slice(1)
export const emptyAudience = () => ({ confirmed: true, roles: [] as ContentRole[], teams: [] as string[] })
export function contentTemplates(briefs: components['schemas']['TemplateRoleBrief'][], pages: ContentPage[]): ContentTemplate[] {
  const doc = (title: string, markdown: string): ContentDocument => ({ title, markdown, require_acknowledgement: false, audience: emptyAudience(), attachments: [], scheduled_at: null })
  const result: ContentTemplate[] = [
    { id: 'challenge', name: 'Challenge brief', input: { kind: 'challenge', document: doc('The challenge', '# The challenge\n\n{{Describe the problem and the people it affects}}\n\n## What to build\n\n{{Describe the outcome}}\n\n## Success criteria\n\n{{Explain what a good entry demonstrates}}') } },
    { id: 'role', name: 'Role brief', input: { kind: 'brief', document: { ...doc('Mentor brief', '# Your role\n\n{{Describe responsibilities}}\n\n## On the day\n\n{{Add key times and contacts}}'), audience: { confirmed: false, roles: ['mentor'], teams: [] } } } },
    { id: 'rules', name: 'Event rules', input: { kind: 'page', document: { ...doc('Event rules', '# Event rules\n\n{{Describe conduct, competition rules and how changes are communicated}}'), purpose: 'rules', require_acknowledgement: true } } },
    ...(['support', 'conduct'] as const).map(purpose => ({ id: purpose, name: purpose === 'support' ? 'Support contact' : 'Conduct reporting contact', input: { kind: 'page' as const, document: { ...doc(purpose === 'support' ? 'Getting support' : 'Reporting a conduct concern', '# Contact the event team\n\n{{Explain when and how to contact the named people, including availability and external privacy arrangements}}'), purpose, contacts: [{ label: '{{Contact name or team}}', url: '{{Approved contact URL or mailbox}}' }] } } })),
    { id: 'faq', name: 'Frequently asked questions', input: { kind: 'page', document: doc('Frequently asked questions', '# Frequently asked questions\n\n## Where can I get help?\n\n{{Add the support contact}}\n\n## What should I bring?\n\n{{Add equipment and preparation guidance}}') } },
    { id: 'resources', name: 'Resource page', input: { kind: 'page', document: doc('Useful resources', '# Useful resources\n\n{{Add useful links and explain when to use them}}') } },
  ]
  briefs.forEach(b => result.push({ id: b.id, name: `${roleLabel(b.role)} brief from event template`, input: { kind: 'brief', document: { ...doc(`${roleLabel(b.role)} brief`, b.body), audience: { confirmed: b.role === 'attendee', roles: b.role === 'attendee' ? [] : [b.role as ContentRole], teams: [] } } } }))
  pages.filter(p => p.kind !== 'challenge').forEach(p => result.push({ id: `copy-${p.id}`, name: `Copy: ${p.document.title}`, input: { kind: p.kind, document: { ...p.document, title: `${p.document.title} — copy`, attachments: [], scheduled_at: null } } }))
  return result
}
export function contentProblems(d: ContentDocument): string[] {
  const errors: string[] = []
  if (!d.title.trim()) errors.push('Enter a page title.')
  if (!d.markdown.trim() && !d.attachments.length) errors.push('Add page content or an attachment.')
  if (d.title.includes('{{') || d.markdown.includes('{{')) errors.push('Resolve every {{placeholder}} before approval.')
  if (!d.audience.confirmed && !d.audience.roles.length && !d.audience.teams.length) errors.push('Choose at least one release audience.')
  if (d.purpose === 'rules' && !d.require_acknowledgement) errors.push('Rules require acknowledgement of each released version.')
  if (d.purpose === 'support' || d.purpose === 'conduct') {
    if (!d.contacts?.length) errors.push('Add a configured contact before approval.')
    d.contacts?.forEach(c => { if (!c.label.trim() || c.label.includes('{{') || !safeContactURL(c.url)) errors.push('Resolve each contact label and approved URL before approval.') })
  }
  return errors
}
export function safeContactURL(raw: string): boolean {
  if (raw.trim() !== raw) return false
  try { const u = new URL(raw); return !u.username && !u.password && ((['http:', 'https:'].includes(u.protocol) && !!u.hostname) || (u.protocol === 'mailto:' && !u.search && !u.hash && /^[^\s<>@,]+@[^\s<>@,]+$/.test(decodeURIComponent(u.pathname)))) } catch { return false }
}
export function draftPage(input: ContentInput): ContentPage {
  return { id: crypto.randomUUID(), kind: input.kind, document: structuredClone(input.document), version: 0, revision: 0, state: 'draft', released_version: null, released_at: null, attachment_details: [], acknowledged_at: null, can_manage: true, message: '' }
}
export function changedContent(a: ContentDocument, b: ContentDocument) {
  const labels: Record<keyof ContentDocument, string> = { objectives: 'Selected objective and measure versions', rubric: 'Approved common rubric', purpose: 'Page purpose', contacts: 'Contact routes', require_acknowledgement: 'Required acknowledgement', title: 'Title', markdown: 'Page text', audience: 'Release audience', attachments: 'Attachments', scheduled_at: 'Release schedule' }
  return (Object.keys(labels) as (keyof ContentDocument)[]).filter(k => JSON.stringify(a[k]) !== JSON.stringify(b[k])).map(k => labels[k])
}
