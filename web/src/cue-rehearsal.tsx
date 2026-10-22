import { Button } from './components/ui/button'
import { cueIssue, emptyCue, type Cue } from './presentation-model'

export function CuePreview({ cue, label }: { cue?: Cue | null; label: string }) {
  return <section className="cue-preview" aria-label={label}><p className="eyebrow">{label}</p><h3>{cue?.title || 'No cue selected'}</h3>{cue && <><p className="cue-text">{cue.text}</p>{cue.url && <p className="cue-url">{cue.url}</p>}<p className="muted">{cue.duration_seconds} seconds</p>{cue.content_id && <p className="notice">The room withholds this linked cue until its resource is released for the full audience.</p>}</>}</section>
}

export function Rehearsal({ cues, selected, setSelected, change, disabled, saveDisabled, activateDisabled, saved, activate, save }: { cues: Cue[]; selected: number; setSelected: (value: number) => void; change: (value: Cue[]) => void; disabled: boolean; saveDisabled: boolean; activateDisabled: boolean; saved: boolean; activate: () => void; save: () => void }) {
  const current = cues[selected]
  function update(patch: Partial<Cue>) { change(cues.map((cue, i) => i === selected ? { ...cue, ...patch } : cue)) }
  function move(delta: number) { const next = [...cues], other = selected + delta; [next[selected], next[other]] = [next[other], next[selected]]; change(next); setSelected(other) }
  const issue = cues.map(cueIssue).find(Boolean)
  return <section className="panel mc-rehearsal" aria-label="Cue rehearsal"><h3>Rehearsal</h3><p>Editing, selecting and saving rehearsals leave the room display unchanged. Activate the saved rehearsal only when you are ready.</p>
    <ol className="cue-list">{cues.map((cue, i) => <li key={cue.id}><Button variant={selected === i ? 'default' : 'outline'} onClick={() => setSelected(i)} aria-pressed={selected === i}>Rehearse {i + 1}: {cue.title || 'Untitled cue'}</Button></li>)}</ol>
    <div className="actions"><Button variant="outline" disabled={disabled || cues.length >= 200} onClick={() => { change([...cues, emptyCue()]); setSelected(cues.length) }}>Add cue</Button>{current && <><Button variant="outline" disabled={disabled || selected === 0} onClick={() => move(-1)}>Move cue up</Button><Button variant="outline" disabled={disabled || selected === cues.length - 1} onClick={() => move(1)}>Move cue down</Button><Button variant="outline" disabled={disabled} onClick={() => { change(cues.filter((_, i) => i !== selected)); setSelected(Math.max(0, selected - 1)) }}>Remove cue</Button></>}</div>
    {current && <fieldset disabled={disabled}><legend>Rehearsal cue {selected + 1}</legend><label htmlFor="cue-title">Cue title</label><input id="cue-title" maxLength={200} value={current.title} onChange={e => update({ title: e.target.value })} /><label htmlFor="cue-text">Room text</label><textarea id="cue-text" maxLength={4000} value={current.text} onChange={e => update({ text: e.target.value })} /><label htmlFor="cue-url">HTTPS room link</label><input id="cue-url" type="url" maxLength={2048} value={current.url} onChange={e => update({ url: e.target.value })} /><label htmlFor="cue-duration">Cue countdown in seconds</label><input id="cue-duration" type="number" min={0} max={86400} step={1} value={current.duration_seconds} onChange={e => update({ duration_seconds: e.target.valueAsNumber })} />{current.content_id && <p className="notice">This saved cue links an existing resource. The server withholds it unless that resource is released for the room. No private resource details are loaded here.</p>}</fieldset>}
    <CuePreview cue={current} label="Rehearsal preview" />{issue && <p className="field-error">{issue}</p>}
    <div className="actions"><Button disabled={saveDisabled || !!issue || saved} onClick={save}>Save rehearsal</Button><Button disabled={activateDisabled || !saved || !current || !!issue} onClick={activate}>Activate saved rehearsal</Button></div>{!saved && <p className="muted">Save the rehearsal before activation.</p>}
  </section>
}
