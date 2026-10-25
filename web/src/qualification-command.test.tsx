import {useState} from 'react'
import {webcrypto} from 'node:crypto'
import {fireEvent,render,screen,waitFor} from '@testing-library/react'
import {afterEach,expect,test,vi} from 'vitest'
import {Theme} from './components/theme'
import {CommunicationSafety} from './communication-tests'
import {IdentityContext} from './session'
import {PrivateRequestScope} from './private-request-scope'
import {QualificationCommandHistory,QualificationCommandReview,useQualificationCommand} from './qualification-command'
import {originalQualificationReceipt,readQualification,sendQualification} from './qualification-api'
import {qualificationGuidePage} from './styleguide/qualification-fixture'
import vectors from './qualification-fingerprint-vectors.json'
import type {QualificationCommand} from './qualification-model'
import type {Session} from './api/client'

vi.mock('./qualification-api',()=>({originalQualificationReceipt:vi.fn(),readQualification:vi.fn(),sendQualification:vi.fn()}))
afterEach(()=>{vi.resetAllMocks();vi.unstubAllGlobals()})
const identity:Session={person:{id:'11111111-1111-4111-8111-111111111111',display_name:'Synthetic owner',platform_admin:false},draft_scope:'22222222-2222-4222-8222-222222222222',csrf_token:'synthetic-csrf',can_create_events:false}

function setup(){
 vi.stubGlobal('crypto',webcrypto)
 const vector=vectors[0],page=qualificationGuidePage(),parent=new PrivateRequestScope('qualification-test')
 page.revision=vector.revision;page.round.run_id=vector.receipt.run_id;page.round.version=0;page.round.signoff=null;page.current=false
 const refresh=vi.fn(async()=>undefined),ready=vi.fn(),body=vector.command as unknown as QualificationCommand
 vi.mocked(readQualification).mockResolvedValue(page)
 function Owner(){const command=useQualificationCommand({eventID:'event',runID:page.round.run_id,page,parent,paused:false,refresh,assertActionReady:ready});return <><button onClick={event=>void command.prepare(body,<p>Captured private review reason: {body.reason}</p>,event.currentTarget)}>Review initial panel</button><button onClick={()=>void refresh()}>Read current panel facts</button><QualificationCommandReview {...command}/></>}
 function Workspace(){const [visible,show]=useState(true);return <Theme><CommunicationSafety><IdentityContext.Provider value={identity}><button onClick={()=>show(false)}>Close private panel task</button><QualificationCommandHistory eventID="event" runID={page.round.run_id}/>{visible&&<Owner/>}</IdentityContext.Provider></CommunicationSafety></Theme>}
 render(<Workspace/>);return {vector,page,body,refresh,ready}
}
async function confirm(){fireEvent.click(screen.getByText('Review initial panel'));await screen.findByRole('alertdialog',{name:'Review private panel command'});fireEvent.click(screen.getByRole('button',{name:'Confirm reviewed panel command'}))}

test('lost initial write keeps exact version0 identity after private teardown; only matching original acknowledgement settles it',async()=>{
 const {vector,body,refresh,ready}=setup()
 vi.mocked(sendQualification).mockRejectedValueOnce(new TypeError('Synthetic lost response'))
 await confirm();await screen.findByText('Original panel outcome unknown')
 fireEvent.click(screen.getByText('Read current panel facts'))
 expect(refresh).toHaveBeenCalledOnce();expect(screen.getByText('Original panel outcome unknown')).toBeVisible()
 expect(ready).toHaveBeenCalledTimes(2)
 expect(vi.mocked(sendQualification).mock.calls[0][4]).toEqual(body)
 fireEvent.click(screen.getByText('Close private panel task'))
 expect(screen.queryByText(/Captured private review reason/)).toBeNull()
 expect(screen.getByLabelText('Original panel command history')).toHaveTextContent('Expected panel version 0')
 expect(screen.getByLabelText('Original panel command history')).not.toHaveTextContent(body.reason)
 vi.mocked(originalQualificationReceipt).mockResolvedValueOnce({receipt:{...vector.receipt,fingerprint:'f'.repeat(64)}})
 fireEvent.click(screen.getByRole('button',{name:'Read original panel acknowledgement'}))
 expect(await screen.findByRole('alert')).toHaveTextContent('We could not load this page. Please try again.')
 expect(screen.getByLabelText('Original panel command history')).toHaveTextContent('Outcome unknown')
 expect(screen.queryByText(/Original configure panel acknowledged/)).toBeNull()
 expect(screen.getByRole('button',{name:'Read original panel acknowledgement'})).toBeEnabled()
 vi.mocked(originalQualificationReceipt).mockResolvedValueOnce({receipt:vector.receipt})
 fireEvent.click(screen.getByRole('button',{name:'Read original panel acknowledgement'}))
 await screen.findByText(/Original configure panel acknowledged at panel version 1/)
 await waitFor(()=>expect(screen.queryByLabelText('Original panel command history')).toBeNull())
 expect(sendQualification).toHaveBeenCalledOnce()
})

test('deliberate retry retains the original captured body key and event revision without automatic replay',async()=>{
 const {vector,page}=setup()
 vi.mocked(sendQualification).mockRejectedValueOnce(new TypeError('Synthetic lost response')).mockResolvedValueOnce({...page,receipt:vector.receipt})
 await confirm();await screen.findByText('Original panel outcome unknown')
 expect(sendQualification).toHaveBeenCalledOnce()
 fireEvent.click(screen.getByRole('button',{name:'Retry captured original panel request'}))
 await screen.findByText(/Original configure panel acknowledged for request/)
 expect(sendQualification).toHaveBeenCalledTimes(2)
 expect(vi.mocked(sendQualification).mock.calls[1]).toEqual(vi.mocked(sendQualification).mock.calls[0])
 expect(readQualification).toHaveBeenCalledOnce()
})

test('changed current base prevents POST and explains the unsent result inside the still-open review',async()=>{
 const {page}=setup()
 vi.mocked(readQualification).mockResolvedValueOnce({...page,revision:page.revision+1})
 await confirm()
 const review=await screen.findByRole('alertdialog',{name:'Review private panel command'})
 await waitFor(()=>expect(review).toHaveTextContent('Current source or action readiness could not be confirmed. Nothing was sent. Cancel, read current panel facts and review again.'))
 expect(sendQualification).not.toHaveBeenCalled()
 expect(screen.queryByLabelText('Original panel command history')).toBeNull()
 fireEvent.click(screen.getByRole('button',{name:'Cancel'}))
 await waitFor(()=>expect(screen.queryByRole('alertdialog')).toBeNull())
})
