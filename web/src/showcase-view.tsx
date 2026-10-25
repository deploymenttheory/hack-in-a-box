import type { ReactNode } from 'react'
import type { components } from './api/schema.gen'
import { CollectionRecords } from './components/patterns/collection'
import { StatusBadge } from './components/ui/feedback'
import { Icon } from './components/ui/icon'
import type { NavigationTarget } from './components/patterns/model'
import './showcase.css'

export type ReviewedProject = components['schemas']['OutcomeProject']
export const followUpLabels = { proposed: 'Proposed follow-up', reviewing: 'Under review', accepted: 'Accepted follow-up', in_progress: 'Follow-up in progress', closed: 'Follow-up closed' } as const

export function ProjectAwards({ awards }: { awards: ReviewedProject['awards'] }) {
 return awards.length ? <ul className="fleet-showcase-awards">{awards.map(award => <li key={award.prize_id}><Icon name="success" /><span>{award.name}{award.amount && <> · {award.amount} {award.currency}</>}</span></li>)}</ul> : <span>No published award shown</span>
}

export function ShowcaseRecords({ projects, destination, action }: { projects: readonly ReviewedProject[]; destination: (project: ReviewedProject) => NavigationTarget; action: (project: ReviewedProject) => ReactNode }) {
 return <CollectionRecords label="Reviewed projects" rows={projects} rowKey={project => project.id} rowLabel={project => project.problem} destination={destination} columns={[
  { id: 'solution', label: 'Solution', essential: true, cell: project => <p className="fleet-showcase-excerpt">{project.solution}</p> },
  { id: 'event', label: 'Source', cell: project => <span>{project.event_name ?? 'Event name not shared'}{project.team_name && <> · {project.team_name}</>}{project.archived && <><br /><StatusBadge>Archived · Read-only</StatusBadge></>}</span> },
  { id: 'status', label: 'Reviewed follow-up', cell: project => <StatusBadge>{followUpLabels[project.follow_up_status]}</StatusBadge> },
  { id: 'awards', label: 'Published awards', cell: project => <ProjectAwards awards={project.awards} /> },
 ]} actions={action} />
}

export function ShowcaseReading({ project, references, source }: { project: ReviewedProject; references: ReactNode; source?: ReactNode }) {
 return <article className="fleet-showcase-reading">
  <div className="fleet-showcase-byline"><span>{project.event_name ?? 'Event name not shared'}{project.team_name && <> · {project.team_name}</>}</span>{project.archived && <StatusBadge>Archived · Read-only</StatusBadge>}</div>
  <section><h2>Solution</h2><p className="fleet-showcase-prose">{project.solution}</p></section>
  <section><h2>Published awards</h2><ProjectAwards awards={project.awards} /></section>
  <section><h2>Reviewed follow-up</h2><StatusBadge>{followUpLabels[project.follow_up_status]}</StatusBadge><p>This reviewed status describes follow-up. A proposal or accepted action is not evidence of demonstrated adoption. Later private updates do not change this publication.</p>{source}</section>
  <section><h2>Approved references</h2>{references}<p>Source links retain their provider permissions. Reviewed media requires current consent and audience access.</p></section>
 </article>
}
