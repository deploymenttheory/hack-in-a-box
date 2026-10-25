import {webcrypto} from 'node:crypto'
import {expect,test,vi} from 'vitest'
import vectors from './qualification-fingerprint-vectors.json'
import {matchesQualificationReceipt,qualificationFingerprint,type QualificationCommand} from './qualification-model'

test('matches actual accepted native Service.Change receipts with verbatim text and nil/empty evidence',async()=>{
 vi.stubGlobal('crypto',webcrypto)
 try{
  expect(vectors).toHaveLength(3)
  expect(vectors[0].receipt.expected_version).toBe(0)
  expect(vectors[0].command.reason).toContain('<panel> &')
  expect(vectors[1].command.evidence).toEqual([])
  expect(vectors[2].command.evidence).toBeNull()
  for(const vector of vectors){
   const body=vector.command as unknown as QualificationCommand
   expect(await qualificationFingerprint(body,vector.revision)).toBe(vector.receipt.fingerprint)
   const intent={requestID:body.request_id,runID:vector.receipt.run_id,action:body.action,revision:vector.revision,version:body.version,fingerprint:vector.receipt.fingerprint}
   expect(matchesQualificationReceipt(vector.receipt,intent)).toBe(true)
   for(const change of [{request_id:'another'},{run_id:'another'},{action:'another'},{expected_revision:-1},{expected_version:-1},{version:99},{fingerprint:'b'.repeat(64)}])expect(matchesQualificationReceipt({...vector.receipt,...change},intent)).toBe(false)
  }
 }finally{vi.unstubAllGlobals()}
})
