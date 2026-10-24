import type { components } from './api/schema.gen'
import { api, APIError, unwrap } from './api/client'
import type { QueueItem } from './components/patterns'
import type { TodayContext } from './today-view'
import { administrationSectionURL } from './today-navigation'
import { localDate } from './event-time'

type Schema = components['schemas']
export type TodayPage = Schema['TodayPage']
export type PreparationFact = Pick<Schema['PreparationTask'], 'due_at' | 'blocked' | 'overdue' | 'scheduling_issue' | 'unowned'> & { id: string; title: string; ownerID: string | null; ownerRole: string; status: string; completeChecks: number; checks: number; dependencies: number }
export type SubmissionFact = Pick<Schema['SubmissionPage'], 'outcome' | 'can_draft' | 'can_submit' | 'can_review' | 'cutoff' | 'live_start' | 'revision'> & { finalRevision: number | null }
export type RequiredDocumentFact = { id: string; title: string; kind: string; version: number; acknowledged: boolean; previousVersion?: number }
export type TodayFacts = {
  tasks: PreparationFact[]; requiredDocuments: RequiredDocumentFact[]; nextDocumentCursor?: string; guidance: Schema['EventGuidance']; help: Schema['HelpQueue']; registrations: Schema['Registration'][]
  teams?: Schema['TeamPage']; ownTeam?: Schema['HackTeam']; disbandedTeamName?: string; submission?: SubmissionFact
  credentials: Schema['CredentialMetadata'][]; repository?: Schema['OwnRepositoryInvitations']; repositoryPendingRelease: boolean
  mentor?: Schema['MentorPage']; review?: Schema['ReviewCoveragePage']; schedule: Schema['ScheduleItem'][]; content?: Schema['ContentReadiness']
}
async function unavailable<T>(read: Promise<T>): Promise<T | undefined> {
  try { return await read } catch (error) { if (error instanceof APIError && error.status === 404) return undefined; throw error }
}
export async function loadTodayFacts(page: TodayPage, context: TodayContext, personID: string, signal: AbortSignal, reviewCursor?: string, documentCursor?: string, assertCurrent: () => void = () => signal.throwIfAborted()): Promise<TodayFacts> {
  const eventID = page.event.id, path = { event_id: eventID }, options = { signal, cache: 'no-store' as const }
  assertCurrent()
  const [preparation, guidance, help, registration, documents] = await Promise.all([
    api.GET('/api/v1/events/{event_id}/preparation/tasks', { ...options, params: { path } }).then(unwrap),
    api.GET('/api/v1/events/{event_id}/guidance', { ...options, params: { path } }).then(unwrap),
    api.GET('/api/v1/events/{event_id}/help', { ...options, params: { path } }).then(unwrap),
    api.GET('/api/v1/events/{event_id}/registrations/mine', { ...options, params: { path } }).then(unwrap),
    api.GET('/api/v1/events/{event_id}/content', { ...options, params: { path, query: { preview: false, limit: 25, cursor: documentCursor } } }).then(unwrap),
  ])
  assertCurrent()
  const role = context === 'personal' ? page.participation_state === 'confirmed' ? 'attendee' : undefined : context
  const tasks = preparation.items.filter(task => context === 'organiser' && page.event.can_read_staff || task.definition.owner_id === personID || !task.definition.owner_id && !!role && task.definition.owner_role === role).map(task => ({ id: task.definition.id, title: task.definition.title, ownerID: task.definition.owner_id, ownerRole: task.definition.owner_role, status: task.definition.status, due_at: task.due_at, blocked: task.blocked, overdue: task.overdue, scheduling_issue: task.scheduling_issue, unowned: task.unowned, completeChecks: task.definition.checklist.filter(item => item.complete).length, checks: task.definition.checklist.length, dependencies: task.definition.depends_on.length }))
  const requiredDocuments = documents.items.filter(item => item.released_version === item.version && item.document.require_acknowledgement).map(item => ({ id: item.id, title: item.document.title, kind: item.kind, version: item.version, acknowledged: !!item.acknowledged_at, previousVersion: item.previous_acknowledgement?.version }))
  const result: TodayFacts = { tasks, requiredDocuments, nextDocumentCursor: documents.next_cursor ?? undefined, guidance, help, registrations: registration.items, credentials: [], repositoryPendingRelease: !page.event.published_at, schedule: [] }
  if (context === 'personal' && page.participation_state === 'confirmed') {
    const teams = unwrap(await api.GET('/api/v1/events/{event_id}/teams', { ...options, params: { path } })); assertCurrent(); result.teams = teams
    const contains = (team: Schema['HackTeam']) => team.published.some(person => person.id === personID) || team.draft.some(person => person.id === personID)
    const own = teams.teams.find(team => team.state !== 'disbanded' && contains(team)); result.ownTeam = own
    result.disbandedTeamName = teams.teams.find(team => team.state === 'disbanded' && contains(team))?.name
    if (own && own.published.some(person => person.id === personID)) {
      const teamPath = { ...path, team_id: own.id }
      const [submission, credentials, repository] = await Promise.all([
        unavailable(api.GET('/api/v1/events/{event_id}/teams/{team_id}/submission', { ...options, params: { path: teamPath } }).then(unwrap)),
        api.GET('/api/v1/events/{event_id}/credentials', { ...options, params: { path } }).then(unwrap),
        page.event.published_at ? unavailable(api.GET('/api/v1/events/{event_id}/repositories/{team_id}/invitations', { ...options, params: { path: teamPath } }).then(unwrap)) : Promise.resolve(undefined),
      ])
      assertCurrent()
      if (submission) result.submission = { outcome: submission.outcome, can_draft: submission.can_draft, can_submit: submission.can_submit, can_review: submission.can_review, cutoff: submission.cutoff, live_start: submission.live_start, revision: submission.revision, finalRevision: submission.final?.revision ?? null }
      // Non-managers receive only their authorised metadata, with team_ids intentionally redacted.
      result.credentials = credentials.items.filter(item => !credentials.can_manage || item.team_ids.includes(own.id)); result.repository = repository
    }
  }
  if ((context === 'mentor' || context === 'organiser') && page.event.can_read_review_coverage) {
    const [mentor, review] = await Promise.all([
      api.GET('/api/v1/events/{event_id}/mentor-coverage', { ...options, params: { path } }).then(unwrap),
      api.GET('/api/v1/events/{event_id}/review-coverage', { ...options, params: { path, query: { cursor: reviewCursor, limit: 25 } } }).then(unwrap),
    ]); assertCurrent(); result.mentor = mentor; result.review = review
  }
  if (context === 'organiser' && page.event.can_read_staff) {
    const [schedule, content] = await Promise.all([
      api.GET('/api/v1/events/{event_id}/schedule', { ...options, params: { path } }).then(unwrap),
      api.GET('/api/v1/events/{event_id}/content-readiness', { ...options, params: { path } }).then(unwrap),
    ]); assertCurrent(); result.schedule = schedule.items; result.content = content
  }
  assertCurrent(); return result
}

export type TodayDestinations = {
  section: (name: string) => void; availableSections?: readonly string[]; registration: () => void; resource: (id: string) => void
  help: (id?: string) => void; clarifications: () => void; reviews: (teamID?: string) => void
}
const words = (value: string) => value.replaceAll('_', ' ')
export function todayActions(page: TodayPage, context: TodayContext, personID: string, facts: TodayFacts, go: TodayDestinations): QueueItem[] {
  const scope = { event: page.event.name, entity: context === 'personal' ? 'Personal readiness' : `${words(context)} context` }, items: QueueItem[] = []
  const action = (label: string, onSelect: () => void) => ({ label, icon: 'next' as const, onSelect })
  const sectionAction = (label: string, name: string) => {
    if (!go.availableSections || go.availableSections.includes(name)) return action(label, () => go.section(name))
    const href = administrationSectionURL(page.administration_origin, page.event.id, name)
    return href ? { label: `${label} in the administration portal`, icon: 'next' as const, href } : { label: `${label} — administration portal address unavailable`, icon: 'next' as const, disabled: true, onSelect: () => {} }
  }
  if (context === 'personal') {
    for (const r of facts.registrations) items.push({ id: `registration:${r.id}`, title: `${r.audience === 'attendee' ? 'Competitor registration' : r.audience + ' application'}: ${r.offer?.state === 'expired' ? 'offer expired' : words(r.state)}`, description: r.state === 'offered' ? `${r.offer?.state === 'expired' ? 'This offer is expired and cannot confirm a place.' : r.kind === 'attendee' ? 'An unaccepted offer reserves capacity but is not a confirmed competitor.' : 'An offered role is not a current role grant until accepted and recorded.'}${r.offer ? ` Respond by ${localDate(r.offer.expires_at, page.event.configuration.timezone)}. offer state: ${r.offer.state}.` : ''}` : `Current registration revision ${r.revision}.${r.waitlist_position != null ? ` Waitlist position ${r.waitlist_position}.` : ''}`, scope, icon: 'user', status: { label: r.offer?.state === 'expired' ? 'Offer expired' : words(r.state), tone: r.state === 'confirmed' ? 'success' : r.state === 'offered' ? 'warning' : 'neutral' }, action: action('Review registration and offers', go.registration) })
    const own = facts.ownTeam, teams = facts.teams
    if (teams) items.push({ id: 'team-allocation', title: own ? `Your team: ${own.name}` : facts.disbandedTeamName ? `Team disbanded: ${facts.disbandedTeamName}` : teams.frozen ? 'No active team on the frozen roster' : 'Team allocation pending', description: own ? `${own.state === 'published' ? 'Published' : 'Draft'} roster · ${own.published.length} published members. ${teams.frozen || own.frozen ? 'Membership is frozen; a smaller team may continue or withdraw. Team editing does not reopen.' : 'Team composition remains subject to allocation and freeze rules.'}` : facts.disbandedTeamName ? 'The disbanded team repository is unused and unscored. Membership does not reopen after team freeze.' : teams.frozen ? 'No active team was returned on the frozen roster. Contact your organiser; team reassignment does not reopen.' : 'No current team was returned for your participation. Do not infer a team or bypass allocation.', scope: { ...scope, entity: 'Team readiness' }, icon: 'team', status: { label: teams.frozen ? 'Team freeze confirmed' : 'Team allocation not frozen', tone: teams.frozen ? 'neutral' : 'warning' }, action: action('Open current team allocation', go.registration) })
    if (own) {
      const submission = facts.submission
      items.push({ id: 'deliverables', title: submission ? `Team deliverables: ${words(submission.outcome)}` : 'Current team deliverable status is unavailable', description: submission ? `${submission.can_draft ? 'Private working drafts are permitted. ' : 'Working draft access is not currently open. '}${submission.can_submit ? 'formal submission is open for this team. ' : 'formal submission is not open for this team. '}${submission.finalRevision != null ? `Formal retained revision ${submission.finalRevision}. ` : 'No formal retained revision was returned. '}Cutoff ${localDate(submission.cutoff, page.event.configuration.timezone)}. Preparation does not mean formal acceptance or scoring.` : 'Open the owning workspace for a current check. No draft, final entry or completed review is inferred.', scope: { ...scope, entity: 'Team readiness' }, icon: 'code', status: { label: submission?.can_submit ? 'Formal submission open' : submission?.outcome === 'frozen' ? 'Frozen formal output' : 'Formal submission not confirmed open', tone: submission?.can_submit ? 'success' : 'neutral' }, action: sectionAction('Open team drafts and formal deliverables', 'deliverables') })
      const repository = facts.repository, ownGrants = repository?.grants.filter(grant => grant.member.person_id === personID && grant.member.github_id === repository.current_github_id && grant.member.link_revision === repository.link_revision) ?? []
      items.push({ id: 'repository', title: 'Repository access readiness', description: repository ? `${repository.current_github_id == null ? 'No current linked GitHub identity. ' : 'Current GitHub identity is linked. '}${repository.observed_at ? `Provider observation ${localDate(repository.observed_at, page.event.configuration.timezone)}. ` : 'No provider observation time is available. '}${ownGrants.length ? ownGrants.map(grant => `${grant.state}${grant.effect_uncertain ? ' (effect unconfirmed)' : ''}`).join('; ') : 'No own collaborator grant was returned.'} These are current owning-domain observations, not an original command receipt or proof of provider acceptance.` : facts.repositoryPendingRelease ? 'Repository invitations become readable after the actual common challenge release. No earlier access is inferred.' : 'Repository invitations are not available for this team. A missing mapping is not proof of access.', scope: { ...scope, entity: 'Team readiness' }, icon: 'repository', status: { label: ownGrants.some(grant => grant.effect_uncertain || grant.state === 'ambiguous') ? 'Access effect unconfirmed' : ownGrants.some(grant => grant.state === 'accepted') ? 'Accepted collaborator state observed' : 'Repository access needs review', tone: ownGrants.some(grant => grant.effect_uncertain || grant.state === 'ambiguous') ? 'warning' : ownGrants.some(grant => grant.state === 'accepted') ? 'success' : 'warning' }, action: action('Review repository access in your registration', go.registration) })
      for (const credential of facts.credentials) items.push({ id: `credential:${credential.id}`, title: credential.label, description: `Assigned ${words(credential.resource_type)} metadata · version ${credential.version}. No secret is revealed by Today; release/expiry and current membership are checked by the owner.`, scope: { ...scope, entity: 'Team readiness' }, icon: 'shield', status: { label: words(credential.readiness), tone: credential.readiness === 'ready' ? 'success' : 'warning' }, action: sectionAction('Open controlled team access', 'team access') })
    }
  }
  for (const task of facts.tasks.filter(task => !['complete', 'waived'].includes(task.status))) items.push({ id: `preparation:${task.id}`, title: task.title, description: `${task.due_at ? `Due ${localDate(task.due_at, page.event.configuration.timezone)}. ` : 'Deadline not confirmed. '}${task.completeChecks}/${task.checks} preparation checklist items complete.${task.dependencies ? ` ${task.dependencies} preparation dependencies.` : ''} These checks do not add a competition eligibility gate or submit team evidence.`, scope: { ...scope, entity: task.ownerID === personID ? 'Your assigned preparation' : `${words(task.ownerRole)} preparation` }, icon: task.blocked ? 'warning' : 'check', status: { label: task.blocked ? 'Blocked prerequisite' : task.overdue ? 'Overdue — server confirmed' : task.scheduling_issue ? 'Deadline needs review' : words(task.status), tone: task.blocked || task.overdue || task.scheduling_issue ? 'warning' : 'neutral' }, action: sectionAction(context === 'organiser' ? 'Open preparation administration' : 'Open your preparation workspace', context === 'organiser' ? 'preparation' : 'deliverables') })
  for (const document of facts.requiredDocuments.filter(document => !facts.guidance.items.some(item => item.id === document.id))) items.push({ id: `required-document:${document.id}`, title: document.title, description: `Released ${document.kind} · version ${document.version}. ${document.acknowledged ? 'This released version is acknowledged.' : document.previousVersion ? `Your version ${document.previousVersion} acknowledgement does not cover this released version.` : 'Acknowledgement of this released version is required.'} Reading a rubric does not assign or complete a judging task.`, scope: { ...scope, entity: 'Required released document' }, icon: 'book', status: { label: document.acknowledged ? 'Current version acknowledged' : 'Required acknowledgement outstanding', tone: document.acknowledged ? 'success' : 'warning' }, action: action('Read and review the released document', () => go.resource(document.id)) })
  const helpRows = context === 'personal' ? facts.help.items.filter(item => item.team_id === facts.help.request_team?.id) : facts.help.items.filter(item => item.assignee_id === personID || context === 'organiser' && facts.help.can_manage)
  for (const request of helpRows) items.push({ id: `help:${request.id}`, title: `${words(request.category)} help: ${request.status}`, description: `Current team request status.${request.assignee_name ? ` Assigned helper: ${request.assignee_name}.` : ''} Support usage is not an assessment or score.`, scope, icon: 'help', status: { label: request.overdue ? 'Overdue help — server confirmed' : request.status, tone: request.overdue ? 'warning' : 'neutral' }, action: action('Open the current help request', () => go.help(request.id)) })
  items.push({ id: 'clarifications', title: 'Official challenge and rubric clarifications', description: 'Only the current published answer feed is official. Private questions are not shared automatically.', scope, icon: 'info', status: { label: 'Authorised published feed' }, action: action('Open official clarifications', go.clarifications) })
  if (context === 'mentor') {
    const assigned = facts.mentor?.published.filter(assignment => assignment.person_id === personID) ?? []
    for (const assignment of assigned) { const team = facts.mentor?.teams.find(team => team.id === assignment.team_id), review = facts.review?.teams.find(team => team.id === assignment.team_id); items.push({ id: `mentor:${assignment.team_id}:${assignment.slot}`, title: `Assigned ${assignment.slot} mentor: ${team?.name ?? 'Current team'}`, description: `Published assignment ${localDate(assignment.window.start, page.event.configuration.timezone)}–${localDate(assignment.window.end, page.event.configuration.timezone)}. ${review ? `Live checkpoint reviews: ${review.assessment.live.submitted}/${review.assessment.live.required_reviews} submitted; ${review.assessment.live.not_open} not open; ${review.assessment.live.missing} missing.` : 'No checkpoint summary for this team is present on the loaded page.'} Preparation is not completed formal review.`, scope, icon: 'team', status: { label: review?.support.actions.overdue ? 'Overdue assigned action — server confirmed' : 'Published mentor assignment', tone: review?.support.actions.overdue ? 'warning' : 'neutral' }, action: action(review ? 'Open assigned team and checkpoints' : 'Open current assigned review coverage', () => go.reviews(review ? assignment.team_id : undefined)) }) }
    if (!assigned.length) items.push({ id: 'mentor-unassigned', title: facts.mentor ? 'No published mentor assignment was returned' : 'Current mentor assignments are unavailable', description: facts.mentor ? 'Draft coverage does not create an assignment. Ask the organiser to publish the current coverage; preparation tasks remain separate.' : 'The current coverage projection is unavailable in this context. No missing or completed assignment is inferred.', scope, icon: 'info', status: { label: 'Assignment not confirmed' }, action: sectionAction('Open current mentor coverage', 'mentors') })
  }
  if (context === 'judge') items.push({ id: 'judge-unavailable', title: 'Final judging assignments are unavailable in this workspace', description: 'Current judging assignments are not available here. This does not mean no assignments exist. Use your organiser’s current instructions; Today never creates a judging, ranking or score state.', scope, icon: 'shield', status: { label: 'Judging task status unavailable', tone: 'warning' }, action: sectionAction('Open released review guidance', 'resources') })
  if (context === 'organiser' && page.event.can_read_staff) {
    for (const issue of page.event.blockers) items.unshift({ id: `lifecycle:${issue.field}:${issue.code}`, title: issue.message, description: `lifecycle blocker: ${issue.field}. The preparation go decision, common challenge publication and live transition remain separate decisions.`, scope, icon: 'warning', status: { label: 'Required lifecycle decision', tone: 'warning' }, action: sectionAction('Review event readiness', 'overview') })
    for (const item of facts.schedule.filter(item => item.status === 'needs_review')) items.push({ id: `schedule:${item.item.id}`, title: item.item.title, description: `staff schedule requires review · due ${localDate(item.due_at, page.event.configuration.timezone)}. This private schedule is not the published room program.`, scope, icon: 'calendar', status: { label: 'Staff schedule needs review', tone: 'warning' }, action: sectionAction('Open staff schedule review', 'schedule') })
    for (const issue of facts.content?.blockers ?? []) items.push({ id: `content:${issue.field}:${issue.code}`, title: issue.message, description: 'release readiness issue. Approval alone is not actual common challenge publication.', scope, icon: 'book', status: { label: 'Release readiness blocked', tone: 'warning' }, action: sectionAction('Open resource release administration', 'content') })
    if (facts.review) for (const team of facts.review.teams.filter(team => team.coverage_issues.length || team.support.actions.overdue || team.assessment.live.missing_assignment)) items.push({ id: `review:${team.id}`, title: `Review exception: ${team.name}`, description: `${team.coverage_issues.join('; ')} · ${team.support.actions.overdue} overdue support actions · ${team.assessment.live.missing_assignment} missing live review assignments. Support, checkpoint reviews and final frozen assessment remain distinct.`, scope, icon: 'warning', status: { label: 'review exception', tone: 'warning' }, action: action('Open current team review coverage', () => go.reviews(team.id)) })
  }
  if (context === 'mc' && page.event.can_present) items.push({ id: 'mc-program', title: 'Control the published room program', description: 'Today shows only its allowlisted current/next projection. Controller drafts, display pairings and explicit timer actions remain in the MC console.', scope, icon: 'calendar', status: { label: 'Current presentation capability' }, action: sectionAction('Open MC console', 'mc console') })
  if (context === 'logistics' && page.event.can_read_logistics) items.push({ id: 'logistics', title: 'Review current logistics readiness', description: 'Use the current authorised operational projection; private attendee details do not enter Today.', scope, icon: 'list', status: { label: 'Current logistics capability' }, action: sectionAction('Open logistics workspace', 'logistics') })
  return items
}
