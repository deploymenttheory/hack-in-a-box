import {webcrypto} from 'node:crypto'
import { describe, expect, it, vi } from 'vitest'
import { freezeFingerprint, frozenCommitURL, verifiedFreeze, matchesFreezeReceipt, type FreezePage } from './submission-freeze-model'
describe('code evidence guarantees',()=>{
 it('matches a Go encoding/json SHA256 fixture including HTML, Unicode separators and empty slices',async()=>{
  vi.stubGlobal('crypto',webcrypto)
  try {expect(await freezeFingerprint({native_revision:3,submission_revision:1,request_id:'11111111-1111-4111-8111-111111111111',action:'reconcile',source_id:'22222222-2222-4222-8222-222222222222',sha:'a'.repeat(40),reason:'<reviewed> & retained\u2028record',observation_ids:[],push_ids:[],external_evidence:'Synthetic\u2029record'},2)).toBe('7cc15262d6aba601018bd19b7af4d11f6fd625723a4992d230bf2fc18a042add')}finally{vi.unstubAllGlobals()}
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
