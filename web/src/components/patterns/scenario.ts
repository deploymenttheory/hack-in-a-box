// Synthetic owner adapter shared by the guide and the isolated browser harness.
// Production patterns import none of this model and never manufacture receipts.
export type ExampleRecord = { id: string; name: string; status: 'Awaiting review' | 'Request recorded'; revision: number; owner: string }
export type ExampleIntent = { id: string; scopeKey: string; recordID: string; expected: number; note: string }
export type ExampleReceipt = { id: string; commandID: string; scopeKey: string; recordID: string; expected: number }
export type ExampleTransportResult = { kind: 'receipt'; receipt: ExampleReceipt } | { kind: 'latest'; revision: number } | { kind: 'rejected' | 'stale' | 'uncertain' }
export type ExampleMode = 'empty' | 'confirmed' | 'uncertain' | 'latest-only' | 'rejected' | 'stale' | 'read-failure' | 'delayed'
export const exampleRecords: readonly ExampleRecord[] = [
  { id: 'atlas', name: 'Project Atlas', status: 'Awaiting review', revision: 3, owner: 'Morgan Lee' },
  { id: 'beacon', name: 'Project Beacon', status: 'Awaiting review', revision: 5, owner: 'Sam Reed' },
  { id: 'cedar', name: 'Project Cedar', status: 'Awaiting review', revision: 2, owner: 'Alex Kim' },
]
export function exampleReceiptMatches(intent: ExampleIntent, receipt: ExampleReceipt) {
  return receipt.id.length > 0 && receipt.commandID === intent.id && receipt.scopeKey === intent.scopeKey && receipt.recordID === intent.recordID && receipt.expected === intent.expected
}
export function examplePage(records: readonly ExampleRecord[], query: string, cursor: number) {
  // This is the synthetic server side. The rendered client receives one page,
  // and its toolbar never filters that incomplete page itself.
  const matches = records.filter(row => row.name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()))
  return { rows: matches.slice(cursor, cursor + 2), next: cursor + 2 < matches.length ? cursor + 2 : undefined }
}
export interface ExampleTransport {
  stats?: Readonly<{ writes: number; reads: number }>
  list(scopeKey: string, query: string, cursor: number): Promise<{ rows: readonly ExampleRecord[]; next?: number }>
  send(intent: ExampleIntent): Promise<ExampleTransportResult>
  status(intent: ExampleIntent): Promise<ExampleTransportResult>
}
export function createExampleTransport(mode: () => ExampleMode, delay = 120): ExampleTransport & { stats: { writes: number; reads: number }; latest: () => readonly ExampleRecord[] } {
  const receipts = new Map<string, ExampleReceipt>(), scopedRecords = new Map<string, ExampleRecord[]>(), stats = { writes: 0, reads: 0 }
  function recordsFor(scopeKey: string) {
    let records = scopedRecords.get(scopeKey)
    if (!records) {
      const names = scopeKey.startsWith('two:') ? ['Project Iris', 'Project Juniper', 'Project Kite'] : scopeKey.includes(':harbour:') ? ['Project Dune', 'Project Ember', 'Project Fjord'] : exampleRecords.map(row => row.name)
      records = exampleRecords.map((row,index) => ({ ...row, name: names[index] }))
      scopedRecords.set(scopeKey, records)
    }
    return records
  }
  const wait = async () => { await new Promise(resolve => setTimeout(resolve, mode() === 'delayed' ? 900 : delay)) }
  return {
    stats, latest: () => [...scopedRecords.values()].flat(),
    async list(scopeKey, query, cursor) { await wait(); if (mode() === 'read-failure') throw new Error('Synthetic current-authority read unavailable'); if (mode() === 'empty') return { rows: [] }; return examplePage(recordsFor(scopeKey), query, cursor) },
    async send(intent) {
      stats.writes++; const selectedMode = mode(); await wait()
      const current = recordsFor(intent.scopeKey).find(row => row.id === intent.recordID)
      if (selectedMode === 'rejected' || !current) return { kind: 'rejected' }
      if (selectedMode === 'stale') { if (current.revision === intent.expected) current.revision++; return { kind: 'stale' } }
      if (current.revision !== intent.expected) return { kind: 'stale' }
      const receipt = { id: 'receipt-' + intent.id, commandID: intent.id, scopeKey: intent.scopeKey, recordID: intent.recordID, expected: intent.expected }
      receipts.set(intent.id, receipt)
      const record = recordsFor(intent.scopeKey).find(row => row.id === intent.recordID)
      if (record) { record.status = 'Request recorded'; record.revision = intent.expected + 1 }
      // Recording a request is deliberately separate from any provider effect
      // or domain completion. The fixture contains neither of those operations.
      return selectedMode === 'uncertain' || selectedMode === 'latest-only' ? { kind: 'uncertain' } : { kind: 'receipt', receipt }
    },
    async status(intent) {
      stats.reads++; await wait()
      if (mode() === 'latest-only') return { kind: 'latest', revision: intent.expected + 1 }
      const receipt = receipts.get(intent.id)
      return receipt ? { kind: 'receipt', receipt } : { kind: 'uncertain' }
    },
  }
}

// The judging example uses fictional fixed identities and no production owner.
export const judgeGuideCriteria: import('../../judge-model').JudgeCriteria = {
 name:'Synthetic judging rubric',mode:'all_teams',required_judge_count:2,finalist_count:3,reserve_count:1,prizes:[],side_quests:[],
 categories:[{id:'8ffbb915-b8cc-439b-b025-00ca75dcf07a',label:'Demonstration',min_score:1,max_score:10,weight:1,guidance:'Explain the observed fictional workflow.',anchors:[{score:1,description:'No working demonstration.'},{score:5,description:'Main workflow has gaps.'},{score:10,description:'Complete demonstrated workflow.'}]}],
}
export const judgeGuideBrief: import('../../judge-model').JudgeBrief = {
 team_id:'3f8af336-a168-47ed-b67c-cba51d530413',team_name:'Fictional practice robot',run_id:'02054da1-8a61-496b-9f97-ff0fc7e1ca61',criteria:judgeGuideCriteria,reference:{id:'d05048ab-0b3e-4dbb-beb6-4371d589e543',version:2,digest:'synthetic-guide-only'},private_judge_guidance:'Use only your independently observed synthetic evidence.',submission_revision:3,
 submission:{definitions:[],evidence:[{deliverable_id:'dfb65eec-fc77-4e7b-86e6-ecb349db03e0',kind:'url',label:'Fictional demonstration',url:'',file_id:null}]},
 repository:{source_id:'00000000-0000-0000-0000-000000000000',status:'not_applicable',sha:'',decision_id:null},own_card:null,submitted_cards:[],deliberation:false,mentor_summary:null,
 output:{team_id:'3f8af336-a168-47ed-b67c-cba51d530413',entry_id:'5fe196de-a63c-4542-93da-4ad4d27299dc',outcome:'frozen',frozen_at:'2027-06-20T15:00:00Z',submitted_at:'2027-06-20T14:00:00Z',frozen_revision:3,artifacts:[]},
}
