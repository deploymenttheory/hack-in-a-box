import { afterEach, describe, expect, it, vi } from 'vitest'
import { webcrypto } from 'node:crypto'
import { judgeFingerprint, matchJudgeReceipt, scoreInputs, validateJudgeScores, type JudgeCommand, type JudgePracticeCommand, type JudgeReceipt } from './judge-model'
import { judgeGuideCriteria } from './components/patterns/scenario'
afterEach(()=>vi.unstubAllGlobals())
const requestID='22222222-2222-4222-8222-222222222222',actorID='1ca7d59b-3160-48b7-99b7-aad2cb648d64'
describe('independent judge input and native original identity',()=>{
 it('retains incomplete input, validates required categories and preserves additional native evidence',()=>{
 const input=scoreInputs(judgeGuideCriteria,[{category_id:judgeGuideCriteria.categories[0].id,value:7,evidence:[{deliverable_id:null,note:'First independent observation'},{deliverable_id:null,note:'Second retained observation'}]}])
 expect(validateJudgeScores(judgeGuideCriteria,input,true).scores[0].evidence).toHaveLength(2)
 input[0].value='';expect(validateJudgeScores(judgeGuideCriteria,input,true).errors[input[0].categoryID]).toContain('Choose a score');expect(input[0].note).toBe('First independent observation')
 expect(validateJudgeScores(judgeGuideCriteria,input,false).errors).toEqual({})
 input[0].value='11';expect(validateJudgeScores(judgeGuideCriteria,input,false).errors[input[0].categoryID]).toContain('1 to 10')
 })
 it('matches Go-marshalled actual native command fingerprints including HTML, unicode and nil fields',async()=>{
 vi.stubGlobal('crypto',webcrypto)
 const scores=[{category_id:judgeGuideCriteria.categories[0].id,value:7,evidence:[{deliverable_id:null,note:'Independent <demo> & review\u2028😀'}]}]
 const practice:JudgePracticeCommand={request_id:requestID,action:'submit',scores}
 const final:JudgeCommand={...practice,action:'submit',submission_revision:3}
 // Generated with actual judging.PracticeCommand/CardCommand through encoding/json+sha256.
 expect(await judgeFingerprint(practice,2)).toBe('38b7eebb367e0a68a6d967a96f0fdd0bea28db5fe00af25b74601fee4db20eb5')
 expect(await judgeFingerprint(final,2,requestID,requestID)).toBe('23565cb9ad6847f87d364bb9707000e7a495c5f9237a7d51dcc5641fcb498e0c')
 })
 it('distinguishes fictional practice receipts from production and rejects mismatched original metadata',()=>{
 const intent={requestID,action:'submit' as const,revision:2,teamID:requestID,fingerprint:'a'.repeat(64)}
 const receipt:JudgeReceipt={request_id:requestID,actor_id:actorID,run_id:'00000000-0000-0000-0000-000000000000',team_id:requestID,action:'practice_submit',revision:3,state:'submitted',fingerprint:intent.fingerprint}
 expect(matchJudgeReceipt(receipt,intent,actorID)).toBe(true)
 for(const change of [{actor_id:requestID},{team_id:actorID},{request_id:actorID},{revision:4},{action:'submit'},{state:'draft'},{fingerprint:'b'.repeat(64)}])expect(matchJudgeReceipt({...receipt,...change},intent,actorID)).toBe(false)
 expect(matchJudgeReceipt({...receipt,action:'practice_reset',state:'draft'},{...intent,action:'reset'},actorID)).toBe(true)
 })
})
