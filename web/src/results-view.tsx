import { Button } from './components/ui/button'
import type { ReactNode } from 'react'
import { ConfirmationDialog } from './components/ui/dialog'
import { Icon } from './components/ui/icon'
import { resultActionLabel, type ResultAnnouncement, type ResultAward, type ResultCommand } from './results-model'
import './results.css'

// This presentation only accepts the allowlisted native announcement DTO. A
// private tally, source, vote or report cannot be passed to a room/public reader.
export function AnnouncedResults({ items, timezone = 'UTC', room = false }: { items: ResultAnnouncement[]; timezone?: string; room?: boolean }) {
 const latest = (['finalists', 'awards'] as const).flatMap(kind => {
  const item = items.filter(item => item.kind === kind).sort((a, b) => b.version - a.version)[0]
  return item ? [item] : []
 })
 return <section className={room ? 'fleet-announcements fleet-announcements-room' : 'fleet-announcements'} aria-label="Announced results">{!latest.length ? <div className="fleet-results-empty"><Icon name="info" /><h2>Results have not been announced</h2><p>Return here after the organiser announces the outcomes.</p></div> : latest.map(item => <section key={item.id} aria-labelledby={`announcement-${item.id}`}><header><p className="fleet-results-eyebrow">Announced · Version {item.version}</p><h2 id={`announcement-${item.id}`}>{item.kind === 'finalists' ? 'Meet the finalists' : 'Celebrating the award winners'}</h2><p><time dateTime={item.published_at}>{new Intl.DateTimeFormat('en-GB', { timeZone: timezone, dateStyle: 'medium', timeStyle: 'short' }).format(new Date(item.published_at))}</time></p></header>{item.kind === 'finalists' ? <><ul className="fleet-result-lines">{item.finalists.map(team => <li key={team.id}><Icon name="check" /><strong>{team.name}</strong><span>Finalist</span></li>)}</ul>{item.reserves.length > 0 && <><h3>Reserves</h3><ul className="fleet-result-lines">{item.reserves.map(team => <li key={team.id}><strong>{team.name}</strong><span>Reserve</span></li>)}</ul></>}</> : <ul className="fleet-result-lines">{item.awards.map(award => <li key={award.prize_id}><Icon name="check" /><div><h3>{award.name}</h3><p>{award.team.name}</p></div>{award.amount && <span>{award.amount} {award.currency}</span>}</li>)}</ul>}</section>)}</section>
}
export function AwardSummary({ awards, teamName }: { awards: ResultAward[]; teamName: (id: string) => string }) {
 return <ul className="fleet-result-lines">{awards.map(a => <li key={a.prize_id}><div><strong>{a.name}</strong><p>{teamName(a.team_id)}</p></div>{a.amount && <span>{a.amount} {a.currency}</span>}</li>)}</ul>
}
export function ResultReview({ action, version, reason, children, pending, disabled, confirm, cancel, returnFocus }: { action: ResultCommand['action']; version: number; reason: string; children?: ReactNode; pending: boolean; disabled: boolean; confirm: () => void; cancel: () => void; returnFocus: () => HTMLElement | false }) {
 return <ConfirmationDialog open title={`Review: ${resultActionLabel[action].toLowerCase()}`} description={action.startsWith('publish_') ? 'This explicitly releases the captured announcement to the event audience and paired room screens.' : 'This records a private result decision. Announcements and team feedback require their own release.'} confirmLabel={resultActionLabel[action]} pending={pending} disabled={disabled} onConfirm={confirm} onOpenChange={open => { if (!open) cancel() }} finalFocus={returnFocus} printExcluded><p>Result version {version}</p>{children}<p>Recorded reason: {reason}</p></ConfirmationDialog>
}

export function BallotReview({ tieLabel, winner, reason, children, pending, disabled, confirm, cancel, returnFocus }: { tieLabel: string; winner: string; reason: string; children: ReactNode; pending: boolean; disabled: boolean; confirm: () => void; cancel: () => void; returnFocus: () => HTMLElement | false }) {
 return <ConfirmationDialog open title="Review your tie vote" description="Record only your own explicit ballot against these current frozen candidates. This does not confirm an award or release an announcement." confirmLabel="Record reviewed tie vote" pending={pending} disabled={disabled} onConfirm={confirm} onOpenChange={open => { if (!open) cancel() }} finalFocus={returnFocus} printExcluded><p>Selected tie: {tieLabel}</p><p>Your selected winner: {winner}</p>{children}<p>Recorded reason: {reason}</p></ConfirmationDialog>
}

export function BallotTasks({ items, selected, disabled, choose }: { items: { id: string; label: string; progress: string; outcome: string; ownVote: boolean }[]; selected: string; disabled: boolean; choose: (id: string) => void }) {
 return <section aria-label="Assigned tie tasks"><h2>Your assigned ties</h2><ul className="fleet-result-lines">{items.map(item => <li key={item.id}><div><h3>{item.label}</h3><p>{item.progress} · {item.outcome}</p><p>{item.ownVote ? 'Your ballot is recorded; replacement requires a new review.' : 'Your ballot is required.'}</p></div><Button icon="next" variant="outline" aria-pressed={selected === item.id} disabled={disabled} onClick={() => choose(item.id)}>Review {item.label}</Button></li>)}</ul></section>
}
