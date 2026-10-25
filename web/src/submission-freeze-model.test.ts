import vectors from './submission-freeze-fingerprint-vectors.json'
import {webcrypto} from 'node:crypto'
import { describe, expect, it, vi } from 'vitest'
import { freezeFingerprint, frozenCommitURL, verifiedFreeze, matchesFreezeReceipt, type FreezePage, type FreezeCommand } from './submission-freeze-model'
describe('code evidence guarantees',()=>{
 it('matches the corpus verified by the actual native Command/fingerprint codec',async()=>{
  vi.stubGlobal('crypto',webcrypto)
  try {for(const vector of vectors)expect(await freezeFingerprint(vector.command as FreezeCommand,vector.revision)).toBe(vector.expected)}finally{vi.unstubAllGlobals()}
 })
 it('requires a completed current binding to the exact frozen native final',()=>{
  const page={native_outcome:'frozen',status:'completed',binding_current:true,current_submission_revision:3,current:{submission_revision:3}} as FreezePage
  expect(verifiedFreeze(page)).toBe(true)
  for(const change of [{status:'unresolved'},{binding_current:false},{native_outcome:'withdrawn'},{current_submission_revision:4},{current:null}])expect(verifiedFreeze({...page,...change})).toBe(false)
 })
 it('constructs only a strict managed GitHub commit link',()=>{
  expect(frozenCommitURL('https://github.com/synthetic-org/synthetic-repo','a'.repeat(40))).toBe('https://github.com/synthetic-org/synthetic-repo/commit/'+'a'.repeat(40))
  for(const repo of ['https://example.test/o/r','https://github.com@evil.example/o/r','https://github.com/o/r/tree/main','javascript:alert(1)','https://github.com/o/r?private=1'])expect(frozenCommitURL(repo,'a'.repeat(40))).toBeUndefined()
  expect(frozenCommitURL('https://github.com/o/r','main')).toBeUndefined()
 })
 it('matches immutable original metadata without requiring a now-private decision body',()=>{
  const original={requestID:'original',action:'amend',revision:4,fingerprint:'a'.repeat(64)},receipt={request_id:'original',action:'amend',expected_revision:4,revision:5,fingerprint:'a'.repeat(64),branch:'',decision:null}
  expect(matchesFreezeReceipt(receipt,original)).toBe(true)
  for(const change of [{request_id:'other'},{action:'configure'},{expected_revision:3},{revision:6},{fingerprint:'b'.repeat(64)}])expect(matchesFreezeReceipt({...receipt,...change},original)).toBe(false)
 })
})
