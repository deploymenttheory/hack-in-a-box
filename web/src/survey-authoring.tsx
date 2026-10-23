import { useEffect, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure } from './components/states'
import { FormDefinitionEditor } from './definition-editor'
import { formDefinitionErrors } from './form-questions'
import { wallTime, localDate } from './event-time'
import { same, nativeDenied, unknownOutcome, useActivePage, useSurveyLeaveWarning, type SurveyConfiguration, type SurveyDefinition } from './survey-context'

type SurveyCommand = components['schemas']['SurveyCommand']
type Props = { event: EventRecord; initial: SurveyConfiguration; writable: boolean; onDirty: (dirty: boolean) => void; receive: (value: SurveyConfiguration) => void; deny: (error: unknown) => void }
export function SurveyAuthoring({ event, initial, writable, onDirty, receive, deny }: Props) {
  const identity = useIdentity(), active = useActivePage(), [saved, setSaved] = useState(initial), [definition, setDefinition] = useState(initial.definition)
  const [opens, setOpens] = useState(wallTime(initial.definition.opens_at, event.configuration.timezone)), [closes, setCloses] = useState(wallTime(initial.definition.closes_at, event.configuration.timezone)), [openDST, setOpenDST] = useState(''), [closeDST, setCloseDST] = useState('')
  const [pending, setPending] = useState(false), [failure, setFailure] = useState<unknown>(), [notice, setNotice] = useState(''), [uncertain, setUncertain] = useState<{ action: SurveyCommand['action']; expected: number; definition?: SurveyDefinition }>(), [conflict, setConflict] = useState(false)
  const timingDirty = opens !== wallTime(saved.definition.opens_at, event.configuration.timezone) || closes !== wallTime(saved.definition.closes_at, event.configuration.timezone) || !!openDST || !!closeDST
  const dirty = !same(definition, saved.definition) || timingDirty, changedElsewhere = initial.version !== saved.version || initial.cancelled !== saved.cancelled || initial.published_at !== saved.published_at
  useEffect(() => { onDirty(dirty || !!uncertain); return () => onDirty(false) }, [dirty, uncertain, onDirty])
  useSurveyLeaveWarning(dirty || !!uncertain)
  const locked = pending || !!uncertain || conflict || changedElsewhere || !writable
  function adopt(value: SurveyConfiguration) { setSaved(value); setDefinition(value.definition); setOpens(wallTime(value.definition.opens_at, event.configuration.timezone)); setCloses(wallTime(value.definition.closes_at, event.configuration.timezone)); setOpenDST(''); setCloseDST(''); setUncertain(undefined); setConflict(false); receive(value) }
  async function readCurrent() { return unwrap(await api.GET('/api/v1/events/{event_id}/surveys/{survey_id}/configuration', { params: { path: { event_id: event.id, survey_id: saved.id } } })) }
  async function act(action: SurveyCommand['action']) {
    setPending(true); setFailure(undefined); setNotice(''); let issued = false
    let intent: { action: SurveyCommand['action']; expected: number; definition?: SurveyDefinition } = { action, expected: saved.version }
    try {
      if (action === 'save') {
        let current = definition
        if (timingDirty) {
          const resolve = async (wall: string, dst: string) => unwrap(await api.POST('/api/v1/time/resolve', { params: { header: { 'X-CSRF-Token': identity.csrf_token } }, body: { wall_time: wall.length === 16 ? wall + ':00' : wall, timezone: event.configuration.timezone, dst_choice: dst as '' | 'earlier' | 'later' } })).instant
          const [opens_at, closes_at] = await Promise.all([resolve(opens, openDST), resolve(closes, closeDST)])
          current = { ...current, opens_at, closes_at }
        }
        intent = { ...intent, definition: current }
      }
      issued = true
      const value = unwrap(await api.PUT('/api/v1/events/{event_id}/surveys/{survey_id}/configuration', { params: { path: { event_id: event.id, survey_id: saved.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${saved.version}"` } }, body: { action, ...(intent.definition ? { definition: intent.definition } : {}) } }))
      if (!active.current) return
      adopt(value); setNotice(action === 'save' ? `Saved question version ${value.version}. Review it before approval.` : action === 'approve' ? `Question version ${value.version} approved, not yet published.` : action === 'publish' ? 'Survey published with fixed invitations. No message has been sent; approve invitation messaging separately.' : 'Survey cancelled. Further response edits and new invitations are stopped; retained authorised history remains.')
    } catch (error) {
      if (!active.current) return
      if (nativeDenied(error)) { deny(error); return }
      setFailure(error)
      if (issued && unknownOutcome(error)) setUncertain(intent)
      else if (issued) setConflict(true)
    } finally { if (active.current) setPending(false) }
  }
  async function reconcile() {
    setPending(true); setFailure(undefined)
    try {
      const current = await readCurrent(); if (!active.current) return
      const match = uncertain && (uncertain.action === 'save' ? current.version === uncertain.expected + 1 && current.author_id === identity.person.id && same(current.definition, uncertain.definition) : current.version === uncertain.expected && (uncertain.action === 'approve' ? !!current.approved_at : uncertain.action === 'publish' ? !!current.published_at : current.cancelled))
      if (match) { adopt(current); setNotice('The current native version confirms the reviewed operation. No second write was sent.') }
      else { setConflict(true); setNotice('The server does not confirm this exact operation. Review its current version before another write.'); receive(current) }
    } catch (error) { if (nativeDenied(error)) deny(error); else setFailure(error) } finally { if (active.current) setPending(false) }
  }
  function reload() { if ((dirty || uncertain) && !window.confirm('Discard these local survey edits and uncertain operation, then load the current native version?')) return; adopt(initial); setFailure(undefined); setNotice('Loaded the current saved configuration.') }
  const errors = formDefinitionErrors(definition.form)
  return <section><h2>Survey questions and publication</h2><p>Draft → exact-version approval → publication. Publication fixes the invitation cohort and questions. Later role grants do not add invitations. A published question set cannot be silently edited.</p><p>Version {saved.version} · {saved.cancelled ? 'Cancelled' : saved.published_at ? 'Published' : saved.approved_at ? 'Approved' : 'Draft'} · Open {localDate(saved.definition.opens_at, event.configuration.timezone)} · Close {localDate(saved.definition.closes_at, event.configuration.timezone)}</p>
    {!saved.published_at && !saved.cancelled ? <fieldset disabled={locked}><legend>Draft definition</legend><FormDefinitionEditor value={definition.form} change={form => setDefinition({ ...definition, form, measures: definition.measures.filter(m => form.fields.some(f => f.id === m.question_id)) })} />
      <label htmlFor="survey-opens">Opens in {event.configuration.timezone}</label><input id="survey-opens" type="datetime-local" step={1} value={opens} onChange={e => setOpens(e.target.value)} /><DSTChoice id="survey-open-dst" value={openDST} change={setOpenDST} label="Repeated opening time" />
      <label htmlFor="survey-closes">Closes in {event.configuration.timezone}</label><input id="survey-closes" type="datetime-local" step={1} value={closes} onChange={e => setCloses(e.target.value)} /><DSTChoice id="survey-close-dst" value={closeDST} change={setCloseDST} label="Repeated closing time" /><p>Times must be on or after the live event ends. Nonexistent times are rejected; repeated times need an explicit occurrence.</p>
      <label htmlFor="survey-cohort">Minimum report cohort (at least 5)</label><input id="survey-cohort" type="number" min={5} max={100} value={definition.minimum_cohort} onChange={e => setDefinition({ ...definition, minimum_cohort: e.target.valueAsNumber })} />
      <SurveyMeasureEditor event={event} definition={definition} change={setDefinition} deny={deny} />
    </fieldset> : <section><h3>Fixed published questions</h3><ol>{saved.definition.form.fields.map(field => <li key={field.id}>{field.label} · {field.type} · {field.required ? 'Required' : 'Optional'}</li>)}</ol></section>}
    {errors.map(error => <p role="alert" key={error}>{error}</p>)}<p role="status" aria-label="Survey configuration status">{notice || (uncertain ? 'The outcome is unknown. Check the exact saved version; this operation will not be sent again automatically.' : dirty ? 'Unsent changes, kept only in this page.' : 'No unsent changes.')}</p>{!!failure && <Failure error={failure} />}
    {(conflict || changedElsewhere) && <p role="alert">The native configuration or authority changed. Review the current version before another action.</p>}
    <div className="actions">{!saved.published_at && !saved.cancelled && <><Button disabled={locked || !dirty && saved.version > 0 || !definition.form.fields.length || !!errors.length || !Number.isInteger(definition.minimum_cohort)} onClick={() => void act('save')}>Save question version</Button><Button variant="outline" disabled={locked || dirty || !saved.version || !!saved.approved_at} onClick={() => void act('approve')}>Approve exact question version</Button><Button disabled={locked || dirty || !saved.approved_at} onClick={() => void act('publish')}>Publish fixed invitations</Button></>}{!saved.cancelled && saved.version > 0 && <Button variant="outline" disabled={locked || dirty} onClick={() => { if (window.confirm('Cancel this survey? New responses and invitations will stop; retained permitted history stays available.')) void act('cancel') }}>Cancel survey</Button>}{uncertain && <Button variant="outline" disabled={pending} onClick={() => void reconcile()}>Check exact saved operation</Button>}{(conflict || changedElsewhere) && <Button variant="outline" disabled={pending} onClick={reload}>Load current survey version</Button>}</div>
  </section>
}
function DSTChoice({ id, value, change, label }: { id: string; value: string; change: (value: string) => void; label: string }) { return <><label htmlFor={id}>{label}</label><select id={id} value={value} onChange={e => change(e.target.value)}><option value="">Require a choice if ambiguous</option><option value="earlier">Earlier occurrence</option><option value="later">Later occurrence</option></select></> }
function SurveyMeasureEditor({ event, definition, change, deny }: { event: EventRecord; definition: SurveyDefinition; change: (value: SurveyDefinition) => void; deny: (error: unknown) => void }) {
  const identity = useIdentity()
  const objectives = useInfiniteQuery({ queryKey: [...privateKey(identity.person.id, event.id, 'survey-objective-options'), identity.draft_scope], gcTime: 0, retry: false, initialPageParam: undefined as string | undefined, queryFn: async ({ pageParam }) => unwrap(await api.GET('/api/v1/events/{event_id}/objectives', { params: { path: { event_id: event.id }, query: { limit: 100, cursor: pageParam } } })), getNextPageParam: page => page.next_cursor ?? undefined })
  useEffect(() => { if (nativeDenied(objectives.error)) deny(objectives.error) }, [objectives.error, deny])
  const options = objectives.data?.pages.flatMap(page => page.items).filter(item => !item.retired).flatMap(item => item.definition.measures.map(measure => ({ key: `${item.reference.id}:${item.reference.version}:${measure.id}`, name: `${item.definition.description} · version ${item.reference.version} · ${measure.description}`, measure: { objective: item.reference, measure_id: measure.id } }))) ?? []
  return <section><h3>Optional exact success-measure references</h3><p>Survey answers are self-reports, not demonstrated adoption. Missing baselines and pending outcome reviews are unknown, never zero.</p>{objectives.error && <Failure error={objectives.error} />}{objectives.hasNextPage && <Button variant="outline" disabled={objectives.isFetchingNextPage} onClick={() => void objectives.fetchNextPage()}>More exact objective definitions</Button>}{definition.form.fields.map(field => {
    const current = definition.measures.find(m => m.question_id === field.id), key = current ? `${current.measure.objective.id}:${current.measure.objective.version}:${current.measure.measure_id}` : ''
    return <div key={field.id}><label htmlFor={`measure-${field.id}`}>Measure for {field.label}</label><select id={`measure-${field.id}`} value={key} onChange={e => { const selected = options.find(o => o.key === e.target.value); change({ ...definition, measures: definition.measures.filter(m => m.question_id !== field.id).concat(selected ? [{ question_id: field.id, measure: selected.measure, construct: current?.construct ?? 'self_reported_learning' }] : []) }) }}><option value="">No linked measure</option>{key && !options.some(o => o.key === key) && <option value={key}>Retained exact objective {current!.measure.objective.id} version {current!.measure.objective.version}</option>}{options.map(option => <option key={option.key} value={option.key}>{option.name}</option>)}</select>{current && <><label htmlFor={`construct-${field.id}`}>Self-report construct</label><select id={`construct-${field.id}`} value={current.construct} onChange={e => change({ ...definition, measures: definition.measures.map(m => m.question_id === field.id ? { ...m, construct: e.target.value as typeof m.construct } : m) })}><option value="self_reported_learning">Self-reported learning</option><option value="self_reported_adoption">Self-reported adoption, not independently evidenced</option></select></>}</div>
  })}</section>
}
