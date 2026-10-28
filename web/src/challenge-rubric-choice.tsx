import { Button } from './components/ui/button'
import { CollectionRecords } from './components/patterns'
import { sameRubric, type ChallengeRubricRead, type ChallengeRubricSnapshot, type RubricReference } from './challenge-rubric-model'
import { rubricReferenceKey } from './rubric-planning-model'

export function ChallengeRubricChoice({ data, value, paused, disabled, choose, configure }: { data: ChallengeRubricRead; value?: RubricReference; paused: boolean; disabled: boolean; choose: (value?: RubricReference) => void; configure: () => void }) {
  const current = data.approved, matches = !!value && !!current && sameRubric(value, current.reference)
  return <section aria-label="Challenge judging criteria" className="grid min-w-0 gap-4">
    <h3>Challenge judging criteria</h3>
    <p>Select an approved snapshot for this challenge. Saving keeps this exact version; a later approval never replaces your choice automatically.</p>
    <p role="status">{paused ? `${value ? `Selected version ${value.version} is retained.` : 'No rubric selected.'} Criteria checks are paused. Your content is retained.` : !value ? 'No rubric selected. You can save a draft, but review and approval need an approved rubric.' : matches ? `Selected version ${value.version} is currently approved.` : `Selected version ${value.version} is retained. Choose the current approved version before review or approval.`}</p>
    {data.selected && <PublicChallengeRubric snapshot={data.selected} label="Selected rubric snapshot" />}
    {current ? <><p>Currently approved: <strong>{current.definition.name}</strong> · Version {current.reference.version} · {current.definition.mode === 'all_teams' ? 'All eligible teams' : 'Mentor qualification, then finalists'}</p>
      {!matches && <PublicChallengeRubric snapshot={current} label="Available approved rubric snapshot" />}
      <div className="fleet-pattern-actions"><Button key={rubricReferenceKey(current.reference)} type="button" icon="check" variant="outline" focusableWhenDisabled={paused && !disabled && !matches} disabled={paused || disabled || matches} onClick={() => choose({ ...current.reference })}>Use approved version {current.reference.version}</Button>
      </div></> : <p>No approved rubric is available. Prepare and approve the criteria first.</p>}
    {value && <div className="fleet-pattern-actions"><Button type="button" icon="remove" variant="outline" focusableWhenDisabled={paused && !disabled} disabled={paused || disabled} onClick={() => choose(undefined)}>Clear rubric selection</Button></div>}
    <Button type="button" icon="next" variant="outline" disabled={disabled} onClick={configure}>Open judging criteria</Button>
    <p className="text-sm text-muted-foreground">Only the exact reference is saved in the challenge. Private judge guidance is excluded from this preview. Selection does not publish criteria or start scoring.</p>
  </section>
}

export function PublicChallengeRubric({ snapshot, label }: { snapshot: ChallengeRubricSnapshot; label: string }) {
  const d = snapshot.definition
  return <details aria-label={label}><summary>{d.name} · Version {snapshot.reference.version} · Public criteria</summary><div className="grid min-w-0 gap-4 py-4">
    <p>{d.mode === 'all_teams' ? 'All eligible teams' : 'Mentor qualification, then finalists'} · Required judges {d.required_judge_count}{d.mode === 'mentor_qualification' && ` · Finalists ${d.finalist_count} · Reserves ${d.reserve_count}`}</p>
    <CollectionRecords label={`${label} categories`} rows={d.categories} rowKey={c => c.id} rowLabel={c => c.label} columns={[
      { id: 'range', label: 'Score bounds', cell: c => `${c.min_score}–${c.max_score}` },
      { id: 'weight', label: 'Weight', cell: c => c.weight },
      { id: 'guidance', label: 'Evidence guidance', cell: c => c.guidance || 'None' },
      { id: 'anchors', label: 'Descriptive anchors', cell: c => <ul>{c.anchors.map(a => <li key={a.score}>{a.score}: {a.description}</li>)}</ul> },
    ]} />
    <h4>Declared awards</h4>{d.prizes.length ? <ul>{d.prizes.map(p => <li key={p.id}>{p.name} · {d.categories.find(c => c.id === p.category_id)?.label ?? 'Overall award'}<p>{p.description}</p></li>)}</ul> : <p>No declared awards.</p>}
    <h4>Evidence side quests</h4>{d.side_quests.length ? <ul>{d.side_quests.map(q => <li key={q.id}>{q.title}<p>{q.evidence_guidance}</p></li>)}</ul> : <p>No side quests.</p>}
  </div></details>
}
