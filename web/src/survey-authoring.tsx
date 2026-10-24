import { useCallback, useEffect, useRef, useState } from 'react'
import { api, APIError, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Field, Input } from './components/ui/fields'
import { SurveyDecisionControls, SurveyDefinitionReview } from './survey-decision'
import { Failure } from './components/states'
import { FormDefinitionEditor } from './definition-editor'
import { FormQuestions, formDefinitionErrors } from './form-questions'
import { wallTime, localDate } from './event-time'
import { CommunicationCommandHistory, useCommunicationCommands } from './communication-tests'
import { usePlanningDepartureBarrier } from './planning-operations'
import { useSurveyRequestScope, useSurveyPrivatePages } from './survey-private-read'
import { same, nativeDenied, unknownOutcome, useSurveyLeaveWarning, type SurveyConfiguration, type SurveyDefinition } from './survey-context'

type SurveyCommand = components['schemas']['SurveyCommand']
type Intent = { action: SurveyCommand['action']; expected: number; definition?: SurveyDefinition; intentID: string }
type Props = { nextAction?: { label: string; onSelect: () => void }; creation?: boolean; event: EventRecord; initial: SurveyConfiguration; writable: boolean; onDirty: (dirty: boolean) => void; receive: (value: SurveyConfiguration) => void; deny: (error: unknown) => void }
export function SurveyAuthoring({ nextAction, creation = false, event, initial, writable, onDirty, receive, deny }: Props) {
  const identity = useIdentity(), owner = `${identity.person.id}:${identity.draft_scope}:${event.id}:${creation ? 'survey-create' : `${initial.id}:survey-configuration`}`, scope = useSurveyRequestScope(owner), history = useCommunicationCommands(owner), lock = useRef(false), opener = useRef<HTMLButtonElement>(null)
  const [saved, setSaved] = useState(initial), [definition, setDefinition] = useState(initial.definition), [section, setSection] = useState('Questions'), [previewAnswers, setPreviewAnswers] = useState<Record<string, unknown>>({})
  const [opens, setOpens] = useState(wallTime(initial.definition.opens_at, event.configuration.timezone)), [closes, setCloses] = useState(wallTime(initial.definition.closes_at, event.configuration.timezone)), [openDST, setOpenDST] = useState(''), [closeDST, setCloseDST] = useState('')
  const [preparing, setPreparing] = useState(false), [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [review, setReview] = useState<Intent>(), [uncertain, setUncertain] = useState<Intent>(), [conflict, setConflict] = useState(false), [current, setCurrent] = useState<SurveyConfiguration>()
  const timingDirty = opens !== wallTime(saved.definition.opens_at, event.configuration.timezone) || closes !== wallTime(saved.definition.closes_at, event.configuration.timezone) || !!openDST || !!closeDST
  const dirty = !same(definition, saved.definition) || timingDirty, changedElsewhere = initial.version !== saved.version || initial.cancelled !== saved.cancelled || initial.published_at !== saved.published_at
  useEffect(() => { onDirty(dirty || !!review || !!uncertain || pending || history.unconfirmed); return () => onDirty(false) }, [dirty, review, uncertain, pending, history.unconfirmed, onDirty])
  useSurveyLeaveWarning(dirty || !!review || !!uncertain, pending || preparing || history.pending)
  usePlanningDepartureBarrier(pending || preparing || history.pending)
  const locked = pending || preparing || history.pending || history.unconfirmed || !!uncertain || conflict || changedElsewhere || !writable
  function adopt(value: SurveyConfiguration) { setSaved(value); setDefinition(value.definition); setOpens(wallTime(value.definition.opens_at, event.configuration.timezone)); setCloses(wallTime(value.definition.closes_at, event.configuration.timezone)); setOpenDST(''); setCloseDST(''); setUncertain(undefined); setConflict(false); setReview(undefined); setCurrent(undefined); setPreviewAnswers({}); receive(value) }
  async function prepare(action: SurveyCommand['action']) {
    if (lock.current || locked || review || action === 'save' && cohortError) return
    lock.current = true; setPreparing(true); setFailure(undefined); setNotice('')
    const ticket = scope.capture()
    try {
      ticket.assert()
      let exact = action === 'save' ? structuredClone(definition) : undefined
      if (exact && timingDirty) {
        const resolve = async (wall: string, dst: string) => unwrap(await api.POST('/api/v1/time/resolve', { signal: ticket.signal, params: { header: { 'X-CSRF-Token': identity.csrf_token } }, body: { wall_time: wall.length === 16 ? wall + ':00' : wall, timezone: event.configuration.timezone, dst_choice: dst as '' | 'earlier' | 'later' } })).instant
        const [opens_at, closes_at] = await Promise.all([resolve(opens, openDST), resolve(closes, closeDST)]); ticket.assert(); exact = { ...exact, opens_at, closes_at }
      }
      ticket.assert(); setReview({ action, expected: saved.version, definition: exact, intentID: crypto.randomUUID() })
    } catch (error) { if (ticket.current()) { if (nativeDenied(error)) deny(error); else setFailure(error) } }
    finally { lock.current = false; if (ticket.current()) setPreparing(false) }
  }
  async function act() {
    if (!review || lock.current || locked) return
    const intent = review, ticket = scope.capture(); if (!ticket.current()) return; lock.current = true; setPending(true); setFailure(undefined)
    const metadataID = history.begin({ operation: `Survey configuration ${intent.action}`, revision: intent.expected, command: { action: intent.action, intentID: intent.intentID, requestID: saved.id, revision: intent.expected } })
    try {
      const value = unwrap(await api.PUT('/api/v1/events/{event_id}/surveys/{survey_id}/configuration', { params: { path: { event_id: event.id, survey_id: saved.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${intent.expected}"` } }, body: { action: intent.action, ...(intent.definition ? { definition: intent.definition } : {}) } }))
      if (value.id !== saved.id || value.version !== intent.expected + (intent.action === 'save' ? 1 : 0) || intent.definition && (!same(value.definition, intent.definition) || value.author_id !== identity.person.id) || intent.action === 'approve' && !value.approved_at || intent.action === 'publish' && !value.published_at || intent.action === 'cancel' && !value.cancelled) throw new Error('Unexpected native survey configuration response')
      history.complete(metadataID); if (!ticket.current()) return
      adopt(value); setNotice(intent.action === 'save' ? `Saved question version ${value.version}. Review it before approval.` : intent.action === 'approve' ? `Question version ${value.version} approved, not yet published.` : intent.action === 'publish' ? 'Survey published with fixed invitations. No message has been sent; approve invitation messaging separately.' : 'Survey cancelled. Further response edits and new invitations are stopped; retained authorised history remains.')
    } catch (error) {
      if (unknownOutcome(error)) history.uncertain(metadataID); else history.complete(metadataID)
      if (!ticket.current()) return
      setReview(undefined); if (nativeDenied(error)) { deny(error); return }; setFailure(error)
      if (unknownOutcome(error)) setUncertain(intent); else if (error instanceof APIError && error.status === 409) setConflict(true)
    } finally { lock.current = false; if (ticket.current()) setPending(false) }
  }
  async function readCurrent() {
    if (lock.current || pending || history.pending) return
    setFailure(undefined); const ticket = scope.capture()
    try { const value = unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/configuration', { signal: ticket.signal, cache: 'no-store', params: { path: { event_id: event.id, survey_id: creation ? history.attempts.at(-1)?.command.requestID ?? saved.id : saved.id } } })); ticket.assert(); setCurrent(value); receive(value); setNotice('Current native configuration read. It does not confirm the original operation; review it before choosing a new base.') }
    catch (error) { if (ticket.current()) { if (nativeDenied(error)) deny(error); else setFailure(error) } }
  }
  function reload() {
    if ((history.unconfirmed || uncertain || conflict) && !current) return
    if ((dirty || uncertain || review) && !window.confirm('Discard these local survey edits and private unknown payload, then use the reviewed current configuration? Earlier unknown metadata remains unconfirmed.')) return
    adopt(current ?? initial); history.selectNewBase(); setFailure(undefined); setNotice('Loaded the current saved configuration. Earlier command outcomes remain unconfirmed.')
  }
  const cohortError = !Number.isInteger(definition.minimum_cohort) || definition.minimum_cohort < 5 || definition.minimum_cohort > 100 ? 'Enter a whole-number report cohort from 5 to 100 people.' : undefined
  const errors = formDefinitionErrors(definition.form), sections = ['Questions', 'Schedule', 'Measures and privacy', 'Preview questions']
  return <section className="fleet-survey-authoring"><h2>Survey questions and publication</h2><p>Draft → review exact version → approve → publish. Publication fixes questions and invitations. Close follows the published native clock; cancellation stops further edits and invitations. Archived events retain only currently authorised history.</p><p>Version {saved.version} · {saved.cancelled ? 'Cancelled' : saved.published_at ? 'Published, fixed questions' : saved.approved_at ? 'Approved, not published' : 'Draft'} · Open {localDate(saved.definition.opens_at, event.configuration.timezone)} · Close {localDate(saved.definition.closes_at, event.configuration.timezone)}</p><CommunicationCommandHistory attempts={history.attempts} />
    {!saved.published_at && !saved.cancelled ? <><nav className="fleet-survey-tabs" aria-label="Survey definition sections">{sections.map(item => <Button key={item} icon={item === 'Questions' ? 'list' : item === 'Schedule' ? 'calendar' : item === 'Preview questions' ? 'book' : 'shield'} variant="ghost" aria-current={section === item ? 'page' : undefined} onClick={() => setSection(item)}>{item}</Button>)}</nav>
      <fieldset className="fleet-survey-fields" disabled={locked || !!review}><legend>Draft definition</legend>
        <div hidden={section !== 'Questions'} inert={section !== 'Questions'}><FormDefinitionEditor value={definition.form} change={form => setDefinition({ ...definition, form, measures: definition.measures.filter(measure => form.fields.some(field => field.id === measure.question_id)) })} /></div>
        <div className="fleet-survey-form" hidden={section !== 'Schedule'} inert={section !== 'Schedule'}><Field label={`Opens in ${event.configuration.timezone}`}><Input type="datetime-local" step={1} value={opens} onChange={event => setOpens(event.target.value)} /></Field><DSTChoice id="survey-open-dst" value={openDST} change={setOpenDST} label="Repeated opening time" /><Field label={`Closes in ${event.configuration.timezone}`}><Input type="datetime-local" step={1} value={closes} onChange={event => setCloses(event.target.value)} /></Field><DSTChoice id="survey-close-dst" value={closeDST} change={setCloseDST} label="Repeated closing time" /><p>Times must be on or after live end. Native resolution rejects nonexistent times; repeated times need an explicit occurrence.</p></div>
        <div hidden={section !== 'Measures and privacy'} inert={section !== 'Measures and privacy'}><Field label="Minimum report cohort (at least 5)" error={cohortError}><Input type="number" min={5} max={100} value={Number.isFinite(definition.minimum_cohort) ? definition.minimum_cohort : ''} onChange={event => setDefinition({ ...definition, minimum_cohort: event.target.valueAsNumber })} /></Field><SurveyMeasureEditor event={event} definition={definition} change={setDefinition} deny={deny} /></div>
        <div hidden={section !== 'Preview questions'} inert={section !== 'Preview questions'}><p>Local question preview only. No response or new survey version is saved.</p><FormQuestions definition={definition.form} answers={previewAnswers} change={setPreviewAnswers} showOptional /></div>
      </fieldset></> : <section className="fleet-survey-section"><h3>Fixed published questions</h3><ol className="fleet-survey-questions">{saved.definition.form.fields.map(field => <li key={field.id}><h4>{field.label}</h4><p>{field.type} · {field.required ? 'Required' : 'Optional'}</p><p>{field.help}</p></li>)}</ol></section>}
    {!!errors.length && !saved.published_at && <ul>{errors.map(error => <li key={error}>{error}</li>)}</ul>}
    <SurveyDecisionControls status={{ label: 'Survey configuration status', message: notice || (uncertain || history.unconfirmed ? 'The operation outcome is unknown. No write is replayed automatically.' : dirty ? 'Unsent draft edits.' : 'Current saved version.') }} actions={[
      ...(saved.published_at && !saved.cancelled && nextAction ? [{ label: nextAction.label, icon: 'next' as const, onSelect: nextAction.onSelect }] : []),
      ...(!saved.published_at && !saved.cancelled ? [
        { label: saved.version ? 'Review draft changes' : 'Review new survey', icon: 'save' as const, disabled: locked || !!errors.length || !!cohortError || !dirty && saved.version > 0, onSelect: (button: HTMLButtonElement) => { opener.current = button; void prepare('save') } },
        { label: 'Review question approval', icon: 'check' as const, variant: 'outline' as const, disabled: locked || dirty || !saved.version || !!saved.approved_at, onSelect: (button: HTMLButtonElement) => { opener.current = button; void prepare('approve') } },
        { label: 'Review survey publication', icon: 'send' as const, variant: 'outline' as const, disabled: locked || dirty || !saved.approved_at, onSelect: (button: HTMLButtonElement) => { opener.current = button; void prepare('publish') } },
      ] : []),
      ...(!saved.cancelled && saved.version > 0 ? [{ label: 'Review survey cancellation', icon: 'close' as const, variant: 'outline' as const, disabled: locked || dirty, onSelect: (button: HTMLButtonElement) => { opener.current = button; void prepare('cancel') } }] : []),
      ...(uncertain || history.unconfirmed || conflict || changedElsewhere ? [{ label: 'Read current survey configuration', icon: 'refresh' as const, variant: 'outline' as const, disabled: pending || preparing || history.pending, onSelect: () => { void readCurrent() } }] : []),
      ...(dirty || uncertain || history.unconfirmed || conflict || changedElsewhere ? [{ label: 'Load current saved configuration', variant: 'outline' as const, disabled: pending || preparing || history.pending || (history.unconfirmed || !!uncertain || conflict) && !current, onSelect: reload }] : []),
    ]} review={{ open: !!review, onOpenChange: open => { if (!open && !pending) setReview(undefined) }, title: 'Review survey configuration action', description: review?.action === 'publish' ? 'Publication fixes this exact question set and invitation cohort. No invitation message is sent by this action.' : review?.action === 'cancel' ? 'Cancellation stops new responses and invitation changes. Retained authorised history remains subject to privacy retention.' : 'Confirm the exact version, questions, timing and privacy below before sending one native command.', confirmLabel: review?.action === 'save' ? 'Save survey draft' : review?.action === 'approve' ? 'Approve exact question version' : review?.action === 'publish' ? 'Publish fixed survey' : 'Cancel survey', pending, disabled: preparing || history.pending || history.unconfirmed || !writable, finalFocus: opener, onConfirm: () => { void act() }, children: <><p>Action {review?.action} · Expected version {review?.expected}</p><SurveyDefinitionReview definition={review?.definition ?? saved.definition} timezone={event.configuration.timezone} /></> }}>
      {!!failure && <Failure error={failure} />}{current && <section className="fleet-survey-section"><h3>Reviewed current configuration</h3><p>Current version {current.version} · {current.definition.form.name} · {current.published_at ? 'Published' : current.approved_at ? 'Approved' : 'Draft'}. This is a current read, not an exact original command receipt.</p></section>}
    </SurveyDecisionControls>
  </section>
}
function DSTChoice({ id, value, change, label }: { id: string; value: string; change: (value: string) => void; label: string }) { return <><label htmlFor={id}>{label}</label><select id={id} value={value} onChange={e => change(e.target.value)}><option value="">Require a choice if ambiguous</option><option value="earlier">Earlier occurrence</option><option value="later">Later occurrence</option></select></> }
function SurveyMeasureEditor({ event, definition, change, deny }: { event: EventRecord; definition: SurveyDefinition; change: (value: SurveyDefinition) => void; deny: (error: unknown) => void }) {
  const identity = useIdentity(), key = `${identity.person.id}:${identity.draft_scope}:${event.id}:survey-objective-options`
  const load = useCallback(async (signal: AbortSignal, cursor?: string) => unwrap(await api.GET('/api/v1/events/{event_id}/objectives', { signal, cache: 'no-store', params: { path: { event_id: event.id }, query: { limit: 100, cursor } } })), [event.id])
  const next = useCallback((page: Awaited<ReturnType<typeof load>>) => page.next_cursor ?? undefined, [])
  const objectives = useSurveyPrivatePages(key, load, next)
  useEffect(() => { if (nativeDenied(objectives.error)) deny(objectives.error) }, [objectives.error, deny])
  const options = objectives.data?.pages.flatMap(page => page.items).filter(item => !item.retired).flatMap(item => item.definition.measures.map(measure => ({ key: `${item.reference.id}:${item.reference.version}:${measure.id}`, name: `${item.definition.description} · version ${item.reference.version} · ${measure.description}`, measure: { objective: item.reference, measure_id: measure.id } }))) ?? []
  return <section><h3>Optional exact success-measure references</h3><p>Survey answers are self-reports, not demonstrated adoption. Missing baselines and pending outcome reviews are unknown, never zero.</p>{!!objectives.error && <Failure error={objectives.error} />}{objectives.hasNextPage && <Button variant="outline" disabled={objectives.isFetchingNextPage} onClick={() => void objectives.fetchNextPage()}>More exact objective definitions</Button>}{definition.form.fields.map(field => {
    const current = definition.measures.find(m => m.question_id === field.id), key = current ? `${current.measure.objective.id}:${current.measure.objective.version}:${current.measure.measure_id}` : ''
    return <div key={field.id}><label htmlFor={`measure-${field.id}`}>Measure for {field.label}</label><select id={`measure-${field.id}`} value={key} onChange={e => { const selected = options.find(o => o.key === e.target.value); change({ ...definition, measures: definition.measures.filter(m => m.question_id !== field.id).concat(selected ? [{ question_id: field.id, measure: selected.measure, construct: current?.construct ?? 'self_reported_learning' }] : []) }) }}><option value="">No linked measure</option>{key && !options.some(o => o.key === key) && <option value={key}>Retained exact objective {current!.measure.objective.id} version {current!.measure.objective.version}</option>}{options.map(option => <option key={option.key} value={option.key}>{option.name}</option>)}</select>{current && <><label htmlFor={`construct-${field.id}`}>Self-report construct</label><select id={`construct-${field.id}`} value={current.construct} onChange={e => change({ ...definition, measures: definition.measures.map(m => m.question_id === field.id ? { ...m, construct: e.target.value as typeof m.construct } : m) })}><option value="self_reported_learning">Self-reported learning</option><option value="self_reported_adoption">Self-reported adoption, not independently evidenced</option></select></>}</div>
  })}</section>
}
