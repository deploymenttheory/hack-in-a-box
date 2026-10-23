export const sections = [
  { id: 'overview', label: 'Principles' },
  { id: 'tokens', label: 'Tokens and themes' },
  { id: 'typography', label: 'Type and spacing' },
  { id: 'components', label: 'Components' },
  { id: 'patterns', label: 'Task patterns' },
  { id: 'qualities', label: '100 quality guidelines' },
  { id: 'contributing', label: 'Making changes' },
] as const
export type Section = typeof sections[number]['id']

export const principles = [
  ['Make work the main content', 'Use a quiet canvas, aligned records and concise reading views. Separate sections with spacing and fine rules. Avoid repeating rounded dashboard panels.'],
  ['Keep the next action clear', 'Use one specific primary action. Keep the event and entity visible through the action, review, result and next step. Important actions have text as well as an icon.'],
  ['Use the appropriate task pattern', 'Use a workspace for general work, an inspector for evidence review and a queue for attention. Reading, galleries and projector content need their own usable composition.'],
  ['Use actual state', 'A saved draft, accepted submission, queued invitation and provider-accepted grant are different facts. Show the owning response; do not infer success from a click, timer or animation.'],
  ['Design for recovery', 'Known failures preserve recoverable input. An unknown effect keeps the original intent and offers an authoritative status read, rather than repeating the write.'],
  ['Keep scope and privacy current', 'Event, account and permission changes invalidate private views and selection. A late response must not restore another account’s data. Never put credentials in a preview or example.'],
] as const

export const contributionSteps = [
  'Find the production primitive or task pattern before introducing new page styling. Keep brand, semantic and component token responsibilities separate.',
  'State the user action and the authoritative API/receipt that proves its result. Preserve CSRF, version checks, validation, current permissions and private draft rules.',
  'Update the production component, interactive example, usage/accessibility guidance and source snippet together when behaviour changes. Avoid maintaining a second example-only implementation.',
  'Exercise keyboard/touch and narrow screens, focus return, loading/empty/no-match, field/transport errors and definite versus unknown outcomes. Keep status meaningful without colour.',
  'Check actual rendered colours under light, dark and supported event branding, including portalled controls. Respect zoom, reduced motion and forced colours.',
  'Run focused meaningful component/browser checks plus repository lint, builds and required CI. Capture synthetic visual evidence and fix defects rather than replacing domain assertions with cosmetic checks.',
] as const

export const tokenGroups = [
  { name: 'Colour roles', variables: ['--background', '--foreground', '--primary', '--primary-foreground', '--primary-hover', '--link', '--muted', '--muted-foreground', '--border', '--input', '--ring', '--destructive', '--warning', '--success'] },
  { name: 'Typography', variables: ['--font-sans', '--text-caption', '--text-body', '--text-section', '--text-page', '--weight-body', '--weight-emphasis', '--line-body', '--line-heading'] },
  { name: 'Spacing and controls', variables: ['--space-1', '--space-2', '--space-3', '--space-4', '--space-6', '--space-8', '--control-radius', '--control-height', '--row-padding', '--page-padding', '--focus-width', '--focus-offset', '--icon-size', '--icon-stroke'] },
  { name: 'Motion', variables: ['--duration-fast', '--duration-normal', '--motion-easing'] },
] as const
