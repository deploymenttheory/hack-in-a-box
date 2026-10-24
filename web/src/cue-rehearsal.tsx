import { Button } from './components/ui/button'
import { cueErrors, cueIssue, emptyCue, type Cue } from './presentation-model'

export { CuePreview } from './cue-preview'
import { CuePreview } from './cue-preview'
import { Field, Input, Textarea } from './components/ui/fields'

export function Rehearsal({ cues, selected, setSelected, change, disabled, saveDisabled, activateDisabled, saved, activate, save }: { cues: Cue[]; selected: number; setSelected: (value: number) => void; change: (value: Cue[]) => void; disabled: boolean; saveDisabled: boolean; activateDisabled: boolean; saved: boolean; activate: () => void; save: () => void }) {
  const current = cues[selected], errors=current?cueErrors(current):{}
  function update(patch: Partial<Cue>) { change(cues.map((cue, i) => i === selected ? { ...cue, ...patch } : cue)) }
  function move(delta: number) { const next = [...cues], other = selected + delta; [next[selected], next[other]] = [next[other], next[selected]]; change(next); setSelected(other) }
  const issue = cues.map(cueIssue).find(Boolean)
  return <section className="fleet-planning-section mc-rehearsal" aria-label="Cue rehearsal"><h3>Rehearsal</h3><p>Editing, selecting and saving rehearsals leave the room display unchanged. Activate the saved rehearsal only when you are ready.</p>
    <ol className="cue-list">{cues.map((cue, i) => <li key={cue.id}><Button variant={selected === i ? 'default' : 'outline'} onClick={() => setSelected(i)} aria-pressed={selected === i}>Rehearse {i + 1}: {cue.title || 'Untitled cue'}</Button></li>)}</ol>
    <div className="actions"><Button icon="add" variant="outline" disabled={disabled || cues.length >= 200} onClick={() => { change([...cues, emptyCue()]); setSelected(cues.length) }}>Add cue</Button>{current && <><Button icon="previousPage" variant="outline" disabled={disabled || selected === 0} onClick={() => move(-1)}>Move cue up</Button><Button icon="nextPage" variant="outline" disabled={disabled || selected === cues.length - 1} onClick={() => move(1)}>Move cue down</Button><Button icon="remove" variant="outline" disabled={disabled} onClick={() => { change(cues.filter((_, i) => i !== selected)); setSelected(Math.max(0, selected - 1)) }}>Remove cue</Button></>}</div>
    {current && <fieldset disabled={disabled}><legend>Rehearsal cue {selected + 1}</legend><Field id="cue-title" label="Cue title" error={errors.title}><Input maxLength={200} value={current.title} onChange={e => update({ title: e.target.value })} /></Field><Field id="cue-text" label="Room text" error={errors.text}><Textarea maxLength={4000} value={current.text} onChange={e => update({ text: e.target.value })} /></Field><Field id="cue-url" label="HTTPS room link" error={errors.url} description="A full HTTPS destination without a username or password."><Input type="url" maxLength={2048} value={current.url} onChange={e => update({ url: e.target.value })} /></Field><Field id="cue-duration" label="Cue countdown in seconds" error={errors.duration_seconds}><Input type="number" min={0} max={86400} step={1} value={current.duration_seconds} onChange={e => update({ duration_seconds: e.target.valueAsNumber })} /></Field>{current.content_id && <p className="notice">This saved cue links an existing resource. The server withholds it unless that resource is released for the room. No private resource details are loaded here.</p>}</fieldset>}
    <CuePreview cue={current} label="Rehearsal preview" />{issue && <p className="field-error">{issue}</p>}
    <div className="actions"><Button icon="save" disabled={saveDisabled || !!issue || saved} onClick={save}>Save rehearsal</Button><Button icon="send" disabled={activateDisabled || !saved || !current || !!issue} onClick={activate}>Activate saved rehearsal</Button></div>{!saved && <p className="muted">Save the rehearsal before activation.</p>}
  </section>
}
