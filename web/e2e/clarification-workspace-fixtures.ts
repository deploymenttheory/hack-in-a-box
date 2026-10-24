import type { Page } from '@playwright/test'
import { fixtureEvent, firstID, mockWorkspace, person, secondID } from './fixtures'
import type { components } from '../src/api/schema.gen'

export const questionID = 'c476b9e4-b36e-430f-984d-d91169e93f15', answerID = '3fa01a65-d6fb-4e5f-b096-b108ec9fa8f1', teamID = '23247975-0951-46c5-ad84-20de89194ae7'
export const privateBody = 'PRIVATE-QUESTION-CANARY-DO-NOT-COPY', officialTitle = 'Reviewed common guidance'
export const feedPath = `/events/${firstID}/clarifications`, questionPath = `/events/${firstID}/questions/${questionID}`
type Draft = components['schemas']['ClarificationConfiguration']
type Published = components['schemas']['PublishedClarification']
type Question = components['schemas']['PrivateQuestion']
export async function clarificationFixture(page: Page, role: 'team' | 'other' | 'staff' = 'team') {
  const events = [fixtureEvent(), fixtureEvent(secondID, 'Second synthetic event')]
  for (const event of events) { event.stage = 'live'; event.published_at = '2027-06-13T09:00:00Z'; event.can_edit = role === 'staff'; event.can_read_staff = role === 'staff' }
  const base = await mockWorkspace(page)
  base.events.splice(0, base.events.length, ...events)
  const source: components['schemas']['ClarificationSource'] = { challenge_id: 'ab996724-64cd-4f97-b198-0b370a8d46ba', challenge_version: 1, rubric: { id: '54eaf3e2-a380-499d-a65f-f177013510cf', version: 1, digest: 'a'.repeat(64) } }
  const state = { allowed: true, expired: false, reviewAllowed: true, archived: false, staffAttachments: false, source, draft: undefined as Draft | undefined, dropAsk: false, dropSave: false, dropPublish: false, failReadAfterSave: false, reviewFailures: 0, rejectSave: false, audience: 2, asks: [] as Record<string, unknown>[], saves: [] as { body: unknown; revision: string | undefined }[], actions: [] as string[], uploads: [] as Record<string, unknown>[], requests: [] as string[], actor: { ...person, platform_admin: false } }
  const question: Question = { id: questionID, team_id: teamID, author_id: person.id, revision: 1, topic: 'challenge', body: privateBody, attachments: ['8119cae1-57ca-42b0-8270-186a9610486e'], source: structuredClone(source), status: 'open', created_at: '2027-06-20T09:00:00Z' }
  const transitions: components['schemas']['QuestionTransition'][] = [{ id: 'f6ce251d-dc9d-44b7-af6d-57975f6eadfa', revision: 1, version: null, action: 'asked', actor_id: person.id, at: question.created_at }]
  function transition(action: components['schemas']['QuestionTransition']['action'], revision: number, version: number | null) { transitions.push({ id: `f6ce251d-dc9d-44b7-af6d-${String(transitions.length + 1).padStart(12, '0')}`, revision, version, action, actor_id: state.actor.id, at: '2027-06-20T10:30:00Z' }) }
  const commands = new Map<string, Question>()
  const published: Published[] = [{ id: 'bfce2548-e496-4f5f-a57b-6649134492b9', version: 1, document: { title: officialTitle, body: 'Identical common guidance for every confirmed attendee.', kind: 'clarification' }, source: structuredClone(source), author_name: 'Synthetic organiser', authored_at: '2027-06-20T09:10:00Z', approved_at: '2027-06-20T09:15:00Z', published_at: '2027-06-20T09:20:00Z', current: true, source_current: true }]
  const workspace = (eventID: string) => ({ source: state.source, archived: state.archived, can_review: role === 'staff' && state.reviewAllowed, can_ask: role !== 'staff' && !state.archived, team_id: role === 'staff' ? null : eventID === firstID && role === 'team' ? teamID : '70d889b1-5ca1-49a1-9589-ad68121d1b20', staff_attachments_open: role === 'staff' && state.staffAttachments, review_audience: role === 'staff' && state.reviewAllowed ? { label: 'Current confirmed attendees', count: state.audience } : null })
  await page.route('**/api/v1/session', route => state.expired ? route.fulfill({ status: 401, json: { error: { code: 'unauthenticated', message: 'Sign in.', request_id: 'synthetic' } } }) : route.fulfill({ json: { person: state.actor, csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false, draft_scope: '8862f9fc-9474-42ad-ab40-012a82f54655' } }))
  await page.route('**/api/v1/events/**', async route => {
    const request = route.request(), url = new URL(request.url()), path = url.pathname, method = request.method()
    state.requests.push(`${method} ${path}`)
    const error = (status = 404) => route.fulfill({ status, json: { error: { code: status === 409 ? 'conflict' : status === 401 ? 'unauthenticated' : 'not_found', message: 'Not currently available.', request_id: 'synthetic' } } })
    const eventID = path.split('/')[4]
    if (path.endsWith('/clarification-workspace')) return !state.allowed || state.expired || (role === 'staff' && !state.reviewAllowed) ? error(state.expired ? 401 : 404) : route.fulfill({ json: workspace(eventID) })
    if (path.includes('/questions') || path.includes('/clarifications') || path.endsWith('/rubric')) {
      if (!state.allowed || state.expired) return error(state.expired ? 401 : 404)
    }
    if (path.endsWith('/questions')) {
      if (method === 'POST') {
        const body = request.postDataJSON(); state.asks.push(body)
        if (role === 'staff' || state.archived || body.team_id !== workspace(eventID).team_id) return error()
        let saved = commands.get(body.command_id)
        if (!saved) { saved = { ...question, id: '0e5b2e41-6c38-4bfe-b490-935da14151ee', body: body.body, topic: body.topic, attachments: body.attachments }; commands.set(body.command_id, saved) }
        if (state.dropAsk) { state.dropAsk = false; return route.abort('failed') }
        return route.fulfill({ json: saved })
      }
      return route.fulfill({ json: { items: eventID === firstID ? [question, ...commands.values()] : [], next_cursor: null } })
    }
    if (path.includes('/questions/')) {
      const id = path.split('/')[6], value = id === questionID ? question : [...commands.values()].find(item => item.id === id)
      if (!value || role === 'other' || (role === 'staff' && !state.reviewAllowed) || eventID !== firstID) return error()
      if (path.endsWith('/review')) { if (state.reviewFailures > 0) { state.reviewFailures--; return error(503) }; return role !== 'staff' || !state.reviewAllowed ? error() : route.fulfill({ json: { draft: state.draft ?? null, workspace: workspace(eventID) } }) }
      if (path.endsWith('/answer')) {
        if (role !== 'staff' || !state.reviewAllowed || state.archived || state.rejectSave) return error(409)
        const body = request.postDataJSON(), revision = request.headers()['if-match']; state.saves.push({ body, revision })
        const next = (state.draft?.version ?? 0) + 1
        if (revision !== `"${state.draft?.revision ?? 0}"`) return error(409)
        state.draft = { id: answerID, question_id: id, revision: (state.draft?.revision ?? 0) + 1, version: next, document: body, source: structuredClone(state.source), approved: false, published: false }
        transition('authored', state.draft.revision, state.draft.version)
        if (state.failReadAfterSave) { state.failReadAfterSave = false; state.reviewFailures = 1 }
        if (state.dropSave) { state.dropSave = false; return route.abort('failed') }
        return route.fulfill({ json: state.draft })
      }
      if (path.endsWith('/triage')) { const action = request.postDataJSON().action; value.status = action === 'triage' ? 'triaged' : action === 'close' ? 'closed' : 'open'; value.revision++; transition(action === 'triage' ? 'triaged' : action === 'close' ? 'closed' : 'reopened', value.revision, null); return route.fulfill({ json: value }) }
      if (path.endsWith('/history')) return route.fulfill({ json: { items: [...transitions].reverse(), next_cursor: null } })
      if (path.includes('/attachments/')) return role === 'staff' && !state.staffAttachments ? error() : route.fulfill({ contentType: 'application/octet-stream', headers: { 'Content-Disposition': 'attachment; filename="private-evidence.txt"', 'Cache-Control': 'no-store' }, body: 'SYNTHETIC-PRIVATE-BYTES' })
      return route.fulfill({ json: value })
    }
    if (path.endsWith('/clarifications')) {
      const search = (url.searchParams.get('search') ?? '').toLowerCase()
      return route.fulfill({ json: { items: published.filter(item => item.current && (item.document.title.toLowerCase().includes(search) || item.document.body.toLowerCase().includes(search))), next_cursor: null } })
    }
    if (path.includes('/clarifications/')) {
      if (path.endsWith('/actions')) {
        if (role !== 'staff' || !state.reviewAllowed || !state.draft || state.archived) return error()
        const body = request.postDataJSON(); state.actions.push(body.action)
        if (body.version !== state.draft.version) return error(409)
        if (body.action === 'approve') state.draft.approved = true
        else { if (!state.draft.approved) return error(409); state.draft.published = true; for (const previous of published) { if (previous.id === state.draft.id) previous.current = false }; published.unshift({ ...published[published.length - 1], id: state.draft.id, version: state.draft.version, document: state.draft.document, source: state.draft.source, current: true, published_at: '2027-06-20T10:30:00Z' }) }
        state.draft.revision++; transition(body.action === 'approve' ? 'approved' : 'published', state.draft.revision, state.draft.version)
        if (body.action === 'publish' && state.dropPublish) { state.dropPublish = false; return route.abort('failed') }
        return route.fulfill({ json: state.draft })
      }
      const history = published.filter(item => item.id === path.split('/')[6])
      if (!history.length) return error()
      return path.endsWith('/history') ? route.fulfill({ json: { items: history, next_cursor: null } }) : route.fulfill({ json: { answer: history[0], history, history_has_more: false, archived: state.archived } })
    }
    if (path.endsWith('/files') && method === 'POST') { const body = request.postDataJSON(); state.uploads.push(body); return route.fulfill({ status: 201, json: { id: '8119cae1-57ca-42b0-8270-186a9610486e', event_id: eventID, owner_id: person.id, filename: body.filename, media_type: body.media_type, size_bytes: 20, checksum: 'synthetic', created_at: '2027-06-20T09:00:00Z' } }) }
    if (path.endsWith('/rubric')) return route.fulfill({ json: { reference: state.source.rubric, published_at: '2027-06-13T09:00:00Z', notice: '', definition: { name: 'Common published expectations', mode: 'all_teams', required_judge_count: 3, finalist_count: 0, reserve_count: 0, prizes: [], side_quests: [], categories: [{ id: '0c6a3aa2-e879-4fca-9f86-12970d2677dd', label: 'Usable demonstration', min_score: 0, max_score: 5, weight: 1, guidance: 'Show working evidence.', anchors: [{ score: 0, description: 'No demonstration' }] }] } } })
    if (path.endsWith('/content')) return route.fulfill({ json: { items: [], next_cursor: null } })
    if (path.includes('/content/')) return route.fulfill({ json: { id: source.challenge_id, kind: 'challenge', revision: 1, version: 1, state: 'released', released_version: 1, released_at: '2027-06-13T09:00:00Z', document: { title: 'Actual common challenge', markdown: 'Common challenge body', audience: { confirmed: true, roles: [], teams: [] }, attachments: [], scheduled_at: null, rubric: source.rubric }, attachment_details: [], acknowledged_at: null, previous_acknowledgement: null } })
    return route.fallback()
  })
  return { state, question, commands, published, events }
}
