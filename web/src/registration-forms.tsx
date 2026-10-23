import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { Controller, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
import { CollectionHeading, CollectionRecords, CollectionPagination, DetailWorkspace } from './components/patterns'
import { AdmissionFeedback, useAdmissionCommand } from './registration-command'
import { usePrivateRequestScope } from './private-request-scope'
import { FormDefinitionEditor } from './definition-editor'
import { FormQuestions, formDefinitionErrors, formEditorSchema, type Answers, type FormDefinition } from './form-questions'

type FormRecord = components['schemas']['RegistrationForm']
export function RegistrationForms({ event, onDirty }: { event: EventRecord; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), [selected, setSelected] = useState<FormRecord>(), [dirty, setDirty] = useState(false)
  const query = useQuery({ queryKey: privateKey(identity.person.id, event.id, 'registration-forms'), queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/registration/forms', { params: { path: { event_id: event.id } } })) })
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  function choose(form: FormRecord) { if (!dirty || window.confirm('Leave this form with unsaved changes?')) setSelected(form) }
  if (query.isPending) return <Loading />
  if (query.error) return <Failure error={query.error} />
  return <section className="fleet-form-workspace"><CollectionHeading title="Registration forms" scope={{ event: event.name, entity: 'Versioned forms' }} description="Publish attendee and facilitator forms separately. Editing a published form creates a new draft; existing answers retain their original version." primary={{ label: 'New registration form', icon: 'add', onSelect: () => choose({ definition: { id: crypto.randomUUID(), name: 'New registration form', audience: 'attendee', fields: [] }, revision: 0, version: 0, published_version: null }) }} /><DetailWorkspace collection={<><CollectionRecords label="Registration forms" rows={query.data.items} rowKey={f => f.definition.id} rowLabel={f => f.definition.name} columns={[{ id: 'audience', label: 'Audience', cell: f => f.definition.audience }, { id: 'version', label: 'Latest version', cell: f => f.version || 'Not saved' }, { id: 'published', label: 'Published version', cell: f => f.published_version ?? 'Not published' }]} actions={f => <Button icon="list" variant="ghost" data-record-focus={f.definition.id} onClick={() => choose(f)}>{f.definition.name} · {f.published_version ? `Published ${f.published_version}` : 'Draft'}</Button>} /><CollectionPagination source={{ kind: 'complete', total: query.data.items.length }} /></>} detail={selected ? <section className="fleet-inspector"><Button icon="back" variant="ghost" onClick={() => { if (!dirty || window.confirm('Leave this form with unsaved changes?')) setSelected(undefined) }}>Back to forms</Button><FormEditor key={selected.definition.id} event={event} initial={selected} onDirty={setDirty} /></section> : undefined} /></section>
}
function FormEditor({ event, initial, onDirty }: { event: EventRecord; initial: FormRecord; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), cache = useQueryClient()
  const [saved, setSaved] = useState(initial), [answers, setAnswers] = useState<Answers>({}), [check, setCheck] = useState<components['schemas']['FormCheck']>(), [ack, setAck] = useState(false), [previewPending, setPending] = useState(false), [notice, setNotice] = useState(''), [failure, setFailure] = useState<unknown>()
  const form = useForm<{ definition: FormDefinition }>({ defaultValues: { definition: initial.definition }, resolver: zodResolver(formEditorSchema), mode: 'onChange' })
  const command = useAdmissionCommand(`${identity.person.id}:${identity.draft_scope}:${event.id}:${initial.definition.id}:form`)
  const scope = usePrivateRequestScope(`${identity.person.id}:${identity.draft_scope}:${event.id}:${initial.definition.id}:preview`), previewLock = useRef(false)
  const pending = previewPending || command.blocked
  const dirty = form.formState.isDirty || command.blocked
  async function readCurrent() {
    const result = unwrap(await api.GET('/api/v1/events/{event_id}/registration/forms', { params: { path: { event_id: event.id } } }))
    const current = result.items.find(item => item.definition.id === initial.definition.id)
    if (current) setSaved(current)
    setNotice('Current form versions refreshed. Your unsaved definition is retained; this read does not establish an unknown write outcome.')
  }
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave this form with unsaved changes?'), enableBeforeUnload: dirty })
  async function action(kind: 'save' | 'publish' | 'preview', definition: FormDefinition) {
    if (pending || previewLock.current) return
    const captured = structuredClone(definition), ticket = scope.capture()
    const params = { path: { event_id: event.id, form_id: captured.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${saved.revision}"` } }
    if (kind === 'preview') {
      previewLock.current = true; setPending(true); setFailure(undefined)
      try { const result = unwrap(await api.POST('/api/v1/events/{event_id}/registration/forms/preview', { params, body: { definition: captured, answers: structuredClone(answers) } })); if (ticket.current()) { setCheck(result); setNotice(result.issues.length ? 'Preview has answers to correct.' : 'Preview answers are valid. Nothing was submitted.') } }
      catch (error) { if (ticket.current()) setFailure(error) }
      finally { if (ticket.current()) { setPending(false); previewLock.current = false } }
      return
    }
    const expected = saved.revision, version = saved.version, acknowledged = ack
    await command.run({ label: kind === 'save' ? 'Save form draft' : 'Publish form version', resource: captured.id, revision: expected }, async () => {
      const result = kind === 'save' ? unwrap(await api.PUT('/api/v1/events/{event_id}/registration/forms/{form_id}', { params, body: captured })) : unwrap(await api.POST('/api/v1/events/{event_id}/registration/forms/{form_id}/publish', { params, body: { version, acknowledge_response_versions: acknowledged } }))
      if (result.definition.id !== captured.id || result.revision <= expected) throw new Error('Unexpected form response')
      return result
    }, async result => {
      setSaved(result); form.reset({ definition: result.definition }); setAck(false)
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'registration-forms') })
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'event') })
      return (kind === 'save' ? `Draft version ${result.version} saved.` : `Version ${result.version} published for new responses.`) + ` Form ${result.definition.id} · revision ${result.revision}.`
    })
  }

  return <form className="fleet-admission-form" onSubmit={event => { void form.handleSubmit(v => action('save', v.definition))(event) }}><h3>Form editor · version {saved.version || 'not saved'}</h3><fieldset className="fleet-admission-fields" disabled={pending}><legend>Form definition</legend><Controller control={form.control} name="definition" render={({ field }) => <><FormDefinitionEditor value={field.value} change={definition => { field.onChange(definition); setCheck(undefined) }} />{formDefinitionErrors(field.value).map(message => <p role="alert" className="field-error" key={message}>{message}</p>)}<section className="fleet-admission-preview"><h3>Try the form</h3><p>Exercise each branch. Validation uses the same server rules as registration.</p><FormQuestions definition={field.value} answers={answers} change={setAnswers} issues={check?.issues} /><Button type="button" variant="outline" pending={pending} disabled={!!formDefinitionErrors(field.value).length} onClick={() => void action('preview', field.value)}>Validate preview answers</Button></section></>} /></fieldset>
    <div className="actions"><Button type="submit" disabled={pending || (saved.version > 0 && !dirty)}>Save form draft</Button></div><section className="fleet-admission-review"><h3>Publication review</h3><p>New required questions apply to responses using the new version. Existing registrations and corrections retain the version originally answered; publishing does not silently invalidate them.</p><label className="check"><input type="checkbox" disabled={pending} checked={ack} onChange={e => setAck(e.target.checked)} />I acknowledge the effect on existing and new responses.</label><Button type="button" disabled={pending || dirty || !saved.version || saved.published_version === saved.version || !ack} onClick={() => void action('publish', saved.definition)}>Publish form version {saved.version}</Button></section><AdmissionFeedback command={command} read={readCurrent} />{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}{failure instanceof APIError && failure.blockers.map(b => <p role="alert" key={b.field}>{b.message}</p>)}</form>
}
