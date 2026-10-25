import type { ReactNode, RefObject } from 'react'
import { ConfirmationDialog } from './components/ui/dialog'
import { StatusBadge } from './components/ui/feedback'
import { ActionLink } from './components/ui/button'
import { localDate } from './event-time'
import { freezeLabel, frozenCommitURL, verifiedFreeze, type FreezePage } from './submission-freeze-model'
import './submission-freeze.css'
export function FreezeEvidence({page,timezone}:{page:FreezePage;timezone:string}) {
  const verified=verifiedFreeze(page),commit=verified&&page.current?frozenCommitURL(page.repository_url,page.current.sha):undefined
  return <section className="fleet-freeze-evidence" aria-label="Code cutoff evidence"><h3>Code cutoff evidence</h3><StatusBadge tone={verified?'success':'neutral'}>{freezeLabel(page)}</StatusBadge><dl className="fleet-freeze-facts">{[['Shared cutoff',localDate(page.cutoff,timezone)],['Configured branch',page.branch||'Not currently disclosed'],['Native final',page.current_submission_revision===null?'None':`Version ${page.current_submission_revision}`],['Repository identity',page.repository_id>0?String(page.repository_id):'No managed numeric repository'],['Current reference',page.current_reference_status.replaceAll('_',' ')]].map(([label,value])=><div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
    {verified&&page.current&&<><p>Reviewed SHA <code>{page.current.sha}</code> · Decision {page.current.id} · Recorded {localDate(page.current.at,timezone)}</p>{commit&&<ActionLink icon="code" href={commit} target="_blank" rel="noopener noreferrer">Open immutable code revision</ActionLink>}</>}
    {!verified&&page.status!=='not_applicable'&&<p>Observed branch heads are candidates, not verified cutoff code. An organiser must review uncertain evidence and record a reasoned decision. A frozen formal submission alone does not verify its code SHA.</p>}
    {page.source&&<p>Original cutoff outcome: {page.source.native_outcome.replaceAll('_',' ')} · Original formal version {page.source.submission_revision??'not disclosed'} · Source recorded {localDate(page.source.recorded_at,timezone)}. Later native corrections retain this original source.</p>}
    {!!page.observations.length&&<><h4>Actual provider observations</h4><ul className="fleet-freeze-history">{page.observations.map(o=><li key={o.id}>{o.status.replaceAll('_',' ')} · Observed {localDate(o.observed_at,timezone)} · {o.provenance}<br/><code>{o.sha||'No SHA observed'}</code><p>Candidate observation {o.id}; observation time does not establish the branch head at cutoff.</p></li>)}</ul></>}
    {!!page.decisions.length&&<details><summary>Authorised immutable decision history</summary><ol className="fleet-freeze-history">{page.decisions.map(d=><li key={d.id}>{d.action==='amend'?'Separate amendment':'Organiser reconciliation'} · Formal version {d.submission_revision} · Recorded {localDate(d.at,timezone)}<br/><code>{d.sha}</code>{d.reason&&<p>{d.reason}</p>}{d.external_evidence&&<p>{d.external_evidence}</p>}<p>Decision {d.id}{d.prior_decision_id&&` · Prior decision ${d.prior_decision_id}`}</p></li>)}</ol></details>}
  </section>
}
export function FreezeReview({title,pending,disabled,confirm,cancel,opener,children}:{title:string;pending:boolean;disabled:boolean;confirm:()=>void;cancel:()=>void;opener:RefObject<HTMLElement|null>;children:ReactNode}) {
 return <ConfirmationDialog open title={title} description="Review the exact current source and effect. The server checks current authority and revisions again at confirmation." confirmLabel="Confirm reviewed code change" pending={pending} disabled={disabled} onConfirm={confirm} onOpenChange={open=>{if(!open)cancel()}} finalFocus={()=>opener.current??false} printExcluded>{children}</ConfirmationDialog>
}
