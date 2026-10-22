import { useState } from 'react'
import { useCurrentTime } from './use-current-time'
import { api, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure } from './components/states'

type Page = components['schemas']['TeamPage']
type Command = components['schemas']['TeamCommand']
type Proposal = components['schemas']['AllocationSuggestion']
type Declaration = components['schemas']['AllocationCandidate']
type Act = (command: Command) => Promise<boolean>

export function TeamConstraints({ team, disabled, act }: { team: Page['teams'][number]; disabled: boolean; act: Act }) {
  const [capacity, setCapacity] = useState(team.capacity?.toString() ?? '')
  return <section className="section-heading" aria-label={`Constraints for ${team.name}`}>
    <p>{team.allocation_locked ? 'Locked cohort: explicitly unlock before moving existing members.' : 'Unlocked for organiser rebalancing.'} {team.capacity ? `Hard capacity: ${team.capacity}.` : 'No hard capacity configured.'}</p>
    <Button variant="outline" disabled={disabled} onClick={() => void act({ action: team.allocation_locked ? 'unlock' : 'lock', team_id: team.id })}>{team.allocation_locked ? 'Unlock team' : 'Lock team'}</Button>
    <label htmlFor={`capacity-${team.id}`}>Hard capacity for {team.name}</label>
    <input id={`capacity-${team.id}`} type="number" min={3} max={10000} value={capacity} disabled={disabled} onChange={e => setCapacity(e.target.value)} />
    <p>Leave empty to clear the hard cap. The preferred four-to-six range is separate.</p>
    <Button variant="outline" disabled={disabled || (capacity !== '' && (!Number.isInteger(Number(capacity)) || Number(capacity) < 3 || Number(capacity) > 10000))} onClick={() => void act({ action: 'capacity', team_id: team.id, capacity: capacity === '' ? null : Number(capacity) })}>Save capacity</Button>
  </section>
}

export function AllocationReview({ eventID, value, disabled, act }: { eventID: string; value: Page; disabled: boolean; act: Act }) {
  const identity = useIdentity()
  const [seed, setSeed] = useState('review-1'), [maximumSize, setMaximumSize] = useState(''), [maximumTeams, setMaximumTeams] = useState('')
  const [declarations, setDeclarations] = useState<Record<string, Declaration>>({})
  const [proposal, setProposal] = useState<Proposal>(), [failure, setFailure] = useState<unknown>(), [loading, setLoading] = useState(false)
  const [review, setReview] = useState<{ revision: number; freeze: boolean }>()
  const teams = value.teams.filter(t => t.state !== 'disbanded')
  const names = new Map(value.people.map(p => [p.id, p.name]))
  const teamNames = new Map(value.teams.map(t => [t.id, t.name]))
  const published = new Map(value.teams.flatMap(t => t.published.map(p => [p.id, t.id] as const)))
  const draft = new Map(teams.flatMap(t => t.draft.map(p => [p.id, t.id] as const)))
  const changes = value.people.filter(p => published.get(p.id) !== draft.get(p.id))
  const unassigned = value.people.filter(p => !draft.has(p.id))
  const currentProposal = proposal?.revision === value.revision ? proposal : undefined
  const currentReview = review?.revision === value.revision ? review : undefined
  const now = useCurrentTime()
  const frozen = value.frozen || now >= new Date(value.freeze_at).getTime()
  function declaration(id: string): Declaration { return declarations[id] ?? { id, skills: [], experience: 0, prefer_with: [] } }
  function update(id: string, patch: Partial<Declaration>) {
    setDeclarations(previous => ({ ...previous, [id]: { ...declaration(id), ...patch } })); setProposal(undefined)
  }
  async function suggest() {
    setLoading(true); setFailure(undefined); setProposal(undefined)
    try {
      const result = unwrap(await api.POST('/api/v1/events/{event_id}/allocation-suggestions', {
        params: { path: { event_id: eventID }, header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${value.revision}"` } },
        body: { seed, limits: { maximum_size: maximumSize ? Number(maximumSize) : null, maximum_teams: maximumTeams ? Number(maximumTeams) : null }, declarations: Object.values(declarations).filter(d => names.has(d.id)) },
      }))
      setProposal(result)
    } catch (error) { setFailure(error) } finally { setLoading(false) }
  }
  return <section className="section-heading" aria-label="Allocation review">
    <h3>Review allocation</h3>
    <p>Suggestions are proposals. Use the placement controls to make reviewed draft changes, then publish explicitly. New proposed groups need a name and captain before publication.</p>
    {unassigned.length > 0 && <p role="status">Unassigned confirmed competitors: {unassigned.map(p => p.name).join(', ')}. Assign everyone before freeze.</p>}
    <details><summary>Declared allocation features</summary><p>Use only competitors’ declared skills, AI-development experience and pairing preferences. Missing declarations remain unknown.</p>
      {value.people.map(p => <fieldset key={p.id} disabled={frozen || loading}><legend>{p.name}</legend>
        <label htmlFor={`skills-${p.id}`}>Declared skills for {p.name}</label><input id={`skills-${p.id}`} value={declaration(p.id).skills.join(', ')} onChange={e => update(p.id, { skills: e.target.value.split(',').map(s => s.trim()) })} />
        <label htmlFor={`experience-${p.id}`}>AI-development experience for {p.name}</label><select id={`experience-${p.id}`} value={declaration(p.id).experience} onChange={e => update(p.id, { experience: Number(e.target.value) })}><option value={0}>Unknown</option><option value={1}>Starting out</option><option value={2}>Some experience</option><option value={3}>Experienced</option></select>
        <label htmlFor={`pairing-${p.id}`}>Preferred teammates for {p.name}</label><select multiple id={`pairing-${p.id}`} value={declaration(p.id).prefer_with} onChange={e => update(p.id, { prefer_with: Array.from(e.target.selectedOptions, o => o.value) })}>{value.people.filter(other => other.id !== p.id).map(other => <option key={other.id} value={other.id}>{other.name}</option>)}</select>
      </fieldset>)}
    </details>
    <label htmlFor="allocation-seed">Suggestion seed</label><input id="allocation-seed" maxLength={200} value={seed} disabled={frozen || loading} onChange={e => { setSeed(e.target.value); setProposal(undefined) }} />
    <label htmlFor="allocation-maximum-size">Proposal maximum team size (optional)</label><input id="allocation-maximum-size" type="number" min={3} max={10000} value={maximumSize} disabled={frozen || loading} onChange={e => { setMaximumSize(e.target.value); setProposal(undefined) }} />
    <label htmlFor="allocation-maximum-teams">Proposal maximum team count (optional)</label><input id="allocation-maximum-teams" type="number" min={1} max={10000} value={maximumTeams} disabled={frozen || loading} onChange={e => { setMaximumTeams(e.target.value); setProposal(undefined) }} />
    <Button variant="outline" disabled={disabled || frozen || loading || !seed.trim() || (!!maximumSize && (!Number.isInteger(Number(maximumSize)) || Number(maximumSize) < 3 || Number(maximumSize) > 10000)) || (!!maximumTeams && (!Number.isInteger(Number(maximumTeams)) || Number(maximumTeams) < 1 || Number(maximumTeams) > 10000))} onClick={() => void suggest()}>{loading ? 'Generating proposal…' : 'Generate allocation proposal'}</Button>
    {proposal && !currentProposal && <p role="status">The roster changed. Generate a fresh proposal before reviewing further moves.</p>}
    {currentProposal && <section aria-label="Allocation proposal"><h4>Allocation proposal · revision {currentProposal.revision}</h4><p>Target {currentProposal.preferences.target}; preferred {currentProposal.preferences.preferred_min}–{currentProposal.preferences.preferred_max}; hard publication minimum {currentProposal.preferences.minimum}.</p>
      {currentProposal.teams.map((t, index) => <article key={t.id}><h4>{teamNames.get(t.id) ?? `Proposed new team ${index + 1}`}{t.locked ? ' · Locked' : ''}</h4><p>{t.members.map(id => names.get(id) ?? id).join(', ')}</p><ul>{t.reasons.map(reason => <li key={reason}>{reason}</li>)}</ul></article>)}
      {currentProposal.retired_teams.length > 0 && <p>Proposed retirements: {currentProposal.retired_teams.map(id => teamNames.get(id) ?? id).join(', ')}. Move survivors. Empty unlocked teams retire when you explicitly publish.</p>}
      {currentProposal.unresolved.length > 0 && <p role="alert">Unresolved competitors: {currentProposal.unresolved.map(id => names.get(id) ?? id).join(', ')}.</p>}
      <ul>{currentProposal.issues.map((issue, i) => <li key={i}>{issue.message}{issue.people.length ? ` (${issue.people.map(id => names.get(id) ?? id).join(', ')})` : ''}</li>)}</ul>
      <Button variant="outline" onClick={() => setProposal(undefined)}>Dismiss proposal</Button>
    </section>}
    {!!failure && <Failure error={failure} />}
    {!frozen && <><div className="actions section-heading"><Button disabled={disabled} onClick={() => setReview({ revision: value.revision, freeze: false })}>Review publication</Button><Button variant="outline" disabled={disabled} onClick={() => setReview({ revision: value.revision, freeze: true })}>Review publication and freeze</Button></div>
      {review && !currentReview && <p role="status">The roster changed. Review the current changes again before publishing.</p>}
      {currentReview && <section className="panel" aria-label="Publication preview"><h4>Publication preview · revision {currentReview.revision}</h4><p>{currentReview.freeze ? 'Publish and freeze every team. Every confirmed competitor must be assigned; no competitor can move afterwards.' : 'Publish these draft assignments. Communication and repository updates follow the published roster.'}</p>
        {changes.length ? <ul>{changes.map(p => <li key={p.id}>{p.name}: {teamNames.get(published.get(p.id) ?? '') ?? 'Unassigned'} → {teamNames.get(draft.get(p.id) ?? '') ?? 'Unassigned'}</li>)}</ul> : <p>No competitor assignment changes.</p>}
        <ul>{teams.map(t => <li key={t.id}>{t.name}: {t.draft.length} members; captain {names.get(t.captain ?? '') ?? 'missing'}{t.draft.length === 0 && !t.allocation_locked ? ' · Will retire at publication' : t.draft.length < 3 ? ' · Below minimum three' : ''}{!t.draft.some(p => p.id === t.captain) ? ' · Captain must be a draft member' : ''}{t.captain !== t.published_captain ? ' · Captain changed' : ''}</li>)}</ul>
        {unassigned.length > 0 && <p>Freeze is blocked until every confirmed competitor is assigned.</p>}
        <div className="actions"><Button disabled={disabled} onClick={() => { if (!currentReview.freeze || window.confirm('Freeze every team now? Every confirmed competitor must be assigned, and no competitor can move afterwards.')) void act({ action: currentReview.freeze ? 'freeze' : 'publish' }).then(saved => { if (saved) setReview(undefined) }) }}>{currentReview.freeze ? 'Publish and freeze teams' : 'Publish assignments'}</Button><Button variant="outline" disabled={disabled} onClick={() => setReview(undefined)}>Cancel publication review</Button></div>
      </section>}
    </>}
  </section>
}
