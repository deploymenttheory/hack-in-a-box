import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { Controller, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { api, APIError, privateKey, unwrap, type EventRecord } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'
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
  return <section><h2>Registration forms</h2><p>Publish attendee and facilitator forms separately. Editing a published form creates a new draft; existing answers retain their original version.</p><div className="actions">{query.data.items.map(f => <Button variant="outline" key={f.definition.id} onClick={() => choose(f)}>{f.definition.name} · {f.published_version ? `Published ${f.published_version}` : 'Draft'}</Button>)}<Button onClick={() => choose({ definition: { id: crypto.randomUUID(), name: 'New registration form', audience: 'attendee', fields: [] }, revision: 0, version: 0, published_version: null })}>New registration form</Button></div>{selected && <FormEditor key={selected.definition.id} event={event} initial={selected} onDirty={setDirty} />}</section>
}
function FormEditor({ event, initial, onDirty }: { event: EventRecord; initial: FormRecord; onDirty: (dirty: boolean) => void }) {
  const identity = useIdentity(), cache = useQueryClient()
  const [saved, setSaved] = useState(initial), [answers, setAnswers] = useState<Answers>({}), [check, setCheck] = useState<components['schemas']['FormCheck']>(), [ack, setAck] = useState(false), [pending, setPending] = useState(false), [notice, setNotice] = useState(''), [failure, setFailure] = useState<unknown>()
  const form = useForm<{ definition: FormDefinition }>({ defaultValues: { definition: initial.definition }, resolver: zodResolver(formEditorSchema), mode: 'onChange' })
  const dirty = form.formState.isDirty
  useEffect(() => { onDirty(dirty); return () => onDirty(false) }, [dirty, onDirty])
  useBlocker({ shouldBlockFn: () => dirty && !window.confirm('Leave this form with unsaved changes?'), enableBeforeUnload: dirty })
  async function action(kind: 'save' | 'publish' | 'preview', definition: FormDefinition) {
    setPending(true); setFailure(undefined); setNotice('')
    try {
      const params = { path: { event_id: event.id, form_id: definition.id }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${saved.revision}"` } }
      if (kind === 'preview') { const result = unwrap(await api.POST('/api/v1/events/{event_id}/registration/forms/preview', { params, body: { definition, answers } })); setCheck(result); setNotice(result.issues.length ? 'Preview has answers to correct.' : 'Preview answers are valid. Nothing was submitted.'); return }
      const result = kind === 'save' ? unwrap(await api.PUT('/api/v1/events/{event_id}/registration/forms/{form_id}', { params, body: definition })) : unwrap(await api.POST('/api/v1/events/{event_id}/registration/forms/{form_id}/publish', { params, body: { version: saved.version, acknowledge_response_versions: ack } }))
      setSaved(result); form.reset({ definition: result.definition }); setAck(false); setNotice(kind === 'save' ? `Draft version ${result.version} saved.` : `Version ${result.version} published for new responses.`)
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'registration-forms') })
      await cache.invalidateQueries({ queryKey: privateKey(identity.person.id, event.id, 'event') })
    } catch (error) { setFailure(error) } finally { setPending(false) }
  }
  return <form className="panel section-heading" onSubmit={form.handleSubmit(v => action('save', v.definition))}><h3>Form editor · version {saved.version || 'not saved'}</h3><Controller control={form.control} name="definition" render={({ field }) => <><FormDefinitionEditor value={field.value} change={definition => { field.onChange(definition); setCheck(undefined) }} />{formDefinitionErrors(field.value).map(message => <p role="alert" className="field-error" key={message}>{message}</p>)}<section className="panel"><h3>Try the form</h3><p>Exercise each branch. Validation uses the same server rules as registration.</p><FormQuestions definition={field.value} answers={answers} change={setAnswers} issues={check?.issues} /><Button type="button" variant="outline" pending={pending} disabled={!!formDefinitionErrors(field.value).length} onClick={() => void action('preview', field.value)}>Validate preview answers</Button></section></>} />
    <div className="actions"><Button type="submit" disabled={pending || (saved.version > 0 && !dirty)}>Save form draft</Button></div><section className="section-heading"><h3>Publication review</h3><p>New required questions apply to responses using the new version. Existing registrations and corrections retain the version originally answered; publishing does not silently invalidate them.</p><label className="check"><input type="checkbox" checked={ack} onChange={e => setAck(e.target.checked)} />I acknowledge the effect on existing and new responses.</label><Button type="button" disabled={pending || dirty || !saved.version || saved.published_version === saved.version || !ack} onClick={() => void action('publish', saved.definition)}>Publish form version {saved.version}</Button></section>{notice && <p role="status">{notice}</p>}{!!failure && <Failure error={failure} />}{failure instanceof APIError && failure.blockers.map(b => <p role="alert" key={b.field}>{b.message}</p>)}</form>
}
