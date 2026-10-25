import {useState,type ReactNode} from 'react'
import {CollectionHeading,CollectionRecords} from './components/patterns'
import {Button,ActionLink} from './components/ui/button'
import {StatusBadge} from './components/ui/feedback'
import {Alert} from './components/ui/surfaces'
import {localDate} from './event-time'
import {currentPrivateSelection,qualificationOutcome,qualificationWords,type QualificationPage,type QualificationCard,type QualificationEvidence,type QualificationProposal} from './qualification-model'
import './qualification.css'

export function QualificationEvidenceList({items}:{items:readonly QualificationEvidence[]}){
 return <ul className="fleet-qualification-evidence">{items.map((item,index)=><li key={`${item.kind}:${item.id}:${item.version}:${index}`}><strong>{qualificationWords(item.kind)}</strong>{item.id&&<> · Source {item.id}</>}{item.version!==null&&<> · Version {item.version}</>}{item.note&&<p>{item.note}</p>}</li>)}{!items.length&&<li>No evidence is disclosed in this current authorised projection. This is not a zero score.</li>}</ul>
}
export function QualificationBlockers({items,label='Current panel blockers'}:{items:readonly string[];label?:string}){
 if(!items.length)return null
 return <Alert role="status" tone="warning"><h3>{label}</h3><ul>{items.map((item,index)=><li key={`${item}:${index}`}>{qualificationWords(item)}</li>)}</ul><p>Resolve the native evidence or assignment requirement, then read the current source before another review.</p></Alert>
}
export function QualificationOverview({page,eventName,teamName,inspect,actions,disabled=false}:{page:QualificationPage;eventName:string;teamName:(id:string)=>string;inspect:(id:string)=>void;actions?:ReactNode;disabled?:boolean}){
 return <section className="fleet-qualification"><CollectionHeading title="Moderation and selections" scope={{event:eventName,entity:'Private post-cutoff panel'}} description="Trace final mentor evidence, record reasoned decisions, then sign the exact selection. Live checkpoints do not accumulate into a final score." status={<StatusBadge tone={page.current?'success':'neutral'}>{page.current?'Current private panel signoff':'Panel signoff incomplete or source changed'}</StatusBadge>} />
 <div className="fleet-pattern-actions">{actions}</div><QualificationBlockers items={page.preview?.blockers??[]}/>
 {!page.preview&&<p>No current selection preview is disclosed. Configure or restore the permitted panel before interpreting missing rankings.</p>}
 <CollectionRecords disabled={disabled} label="Qualification evidence and eligibility" rows={page.teams} rowKey={team=>team.team_id} rowLabel={team=>teamName(team.team_id)} columns={[{id:'outcome',label:'Formal outcome',essential:true,cell:team=>qualificationOutcome(team)},{id:'code',label:'Code evidence',cell:team=>qualificationWords(team.freeze_status)},{id:'review',label:'Review readiness',cell:team=>team.blockers.length?team.blockers.map(qualificationWords).join(' · '):'No team blocker reported'}]} actions={team=><Button focusableWhenDisabled data-record-focus={team.team_id} icon="next" variant="ghost" disabled={disabled} onClick={()=>inspect(team.team_id)}>Inspect {teamName(team.team_id)}</Button>}/>
 <p>Qualification and category decisions stay private. Fresh final judging, final awards, feedback release and public announcements have their own deliberate owners.</p></section>
}
export function QualificationCardComparison({card,page,timezone,source}:{card?:QualificationCard;page:QualificationPage;timezone:string;source:ReactNode}){
 return <div className="fleet-qualification-comparison"><section aria-label="Current native review evidence"><h3>Reviewed source evidence</h3>{source}</section><section aria-label="Moderated final card"><h3>Moderated final card</h3>{!card?<p>No current moderated card is disclosed. Missing evidence is not a zero score.</p>:<><p>Version {card.version} · {qualificationWords(card.state)} · Recorded {localDate(card.changed_at,timezone)}</p><p>Frozen formal version {card.source.submission_revision} · Huddle {card.source.huddle_id} version {card.source.huddle_version} · Rubric version {card.source.rubric.version}</p><p>Recorded reason: {card.reason}</p><dl>{page.criteria.categories.map(category=>{const score=card.scores.find(value=>value.category_id===category.id);return <div key={category.id}><dt>{category.label}</dt><dd>{!score||score.pending||score.value===null?'Pending evidence; unscored':`${score.value} / ${category.max_score}`}<QualificationEvidenceList items={score?.evidence??[]}/></dd></div>})}</dl></>}</section></div>
}
export function QualificationSelection({proposal,teamName,categoryName}:{proposal:QualificationProposal;teamName:(id:string)=>string;categoryName:(id:string)=>string}){
 return <section className="fleet-qualification-selection"><h3>Private selection proposal</h3><QualificationBlockers items={proposal.blockers}/><div className="fleet-qualification-comparison"><section><h4>Proposed finalists</h4><ol>{proposal.finalist_ids.map(id=><li key={id}>{teamName(id)}</li>)}</ol><h4>Reserve</h4><ol>{proposal.reserve_ids.map(id=><li key={id}>{teamName(id)}</li>)}</ol></section><section><h4>All eligible teams remain category candidates</h4><ul>{proposal.categories.map(category=><li key={category.category_id}><strong>{categoryName(category.category_id)}</strong><ul>{category.team_ids.map(id=><li key={id}>{teamName(id)}</li>)}</ul></li>)}</ul></section></div>{proposal.ties.length>0&&<p>{proposal.ties.length} affected ties require evidence review and recorded panel decisions. Ranking alone cannot resolve them.</p>}</section>
}
export function QualificationPreparationPreview({page,teamName,categoryName}:{page:QualificationPage;teamName:(id:string)=>string;categoryName:(id:string)=>string}){
 const [scope,setScope]=useState<'finalists'|'categories'>(),selection=currentPrivateSelection(page)
 if(!selection)return <p>Announcement preparation preview requires a current exact panel signoff. No outcome has been released.</p>
 return <section aria-label="Private announcement preparation preview"><h3>Prepare separately</h3><p>Local private preview only. This does not approve, send or publish an announcement.</p><div className="fleet-pattern-actions"><Button icon="team" variant="outline" onClick={()=>setScope('finalists')}>Preview finalist preparation</Button><Button icon="list" variant="outline" onClick={()=>setScope('categories')}>Preview category preparation</Button></div>{scope&&<section aria-label={scope==='finalists'?'Private finalist preparation':'Private category preparation'}><h4>{scope==='finalists'?'Private finalist preparation':'Private category preparation'}</h4>{scope==='finalists'?<><ol>{selection.finalist_ids.map(id=><li key={id}>{teamName(id)}</li>)}</ol><p>Category selections are intentionally absent from this separate preview.</p></>:<ul>{selection.categories.map(category=><li key={category.category_id}>{categoryName(category.category_id)}: {category.team_ids.map(teamName).join(', ')}</li>)}</ul>}<Button icon="close" variant="ghost" onClick={()=>setScope(undefined)}>Close private preparation preview</Button></section>}</section>
}
export function QualificationEvidenceDestination({eventID,teamID}:{eventID:string;teamID:string}){
 return <ActionLink icon="code" href={`/events/${eventID}/teams/${teamID}/build`}>Inspect formal build and frozen code</ActionLink>
}
