import { CollectionRecords } from './components/patterns'
import type { RubricDefinition } from './rubric-planning-fields'

export function RubricPlanningReview({ definition, revision, version, reason, action }: { definition: RubricDefinition; revision: number; version: number; reason: string; action: 'save' | 'approve' }) {
  return <section aria-label="Captured rubric decision" className="grid gap-4">
    <p>{action === 'save' ? 'Save a new private draft version.' : 'Approve this exact saved version.'} Starting revision {revision} · Version {version}</p>
    <p>Decision reason: {reason}</p><h3>{definition.name}</h3>
    <p>{definition.mode === 'all_teams' ? 'All eligible teams' : 'Mentor qualification, then finalists'} · Required judges {definition.required_judge_count}{definition.mode === 'mentor_qualification' && ` · Finalists ${definition.finalist_count} · Reserves ${definition.reserve_count}`}</p>
    <CollectionRecords label="Reviewed rubric categories" rows={definition.categories} rowKey={category => category.id} rowLabel={category => category.label} columns={[
      { id: 'bounds', label: 'Score bounds', cell: category => `${category.min_score}–${category.max_score}` },
      { id: 'weight', label: 'Weight', cell: category => category.weight },
      { id: 'guidance', label: 'Public evidence guidance', cell: category => category.guidance || 'None' },
      { id: 'anchors', label: 'Descriptive anchors', cell: category => <ul>{category.anchors.map(anchor => <li key={anchor.score}>{anchor.score}: {anchor.description}</li>)}</ul> },
    ]} />
    <h3>Declared awards</h3>{definition.prizes.length ? <ul>{definition.prizes.map(prize => <li key={prize.id}><strong>{prize.name}</strong> · {definition.categories.find(category => category.id === prize.category_id)?.label ?? 'Overall award'}<p>{prize.description}</p></li>)}</ul> : <p>No declared awards.</p>}
    <h3>Evidence side quests</h3>{definition.side_quests.length ? <ul>{definition.side_quests.map(quest => <li key={quest.id}><strong>{quest.title}</strong><p>{quest.evidence_guidance}</p></li>)}</ul> : <p>No side quests.</p>}
    <h3>Private judge guidance</h3><p className="whitespace-pre-wrap">{definition.private_judge_guidance || 'None'}</p>
    <p>Approval does not release these criteria. Common challenge publication uses an exact approved reference and retains its audience, schedule and active-assessment safeguards.</p>
  </section>
}
