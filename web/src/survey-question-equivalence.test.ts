import { describe, expect, it } from 'vitest'
import { equivalentQuestion } from './survey-question-equivalence'
import { canonical, type SurveyConfiguration } from './survey-context'
const question = '2d69935f-f2c5-4f74-b53b-a9d95a7191ca'
function configuration(): SurveyConfiguration {
 return { id: '8d4158fd-3ef9-42eb-9c0c-a064f34db9cd', version: 1, definition: { form: { id: '8d4158fd-3ef9-42eb-9c0c-a064f34db9cd', name: 'Feedback', audience: 'attendee', fields: [{ id: question, label: 'Did this help?', type: 'single', required: true, help: 'Use your own experience.', choices: ['yes', 'no'] }] }, opens_at: '2027-06-21T09:00:00Z', closes_at: '2027-06-28T09:00:00Z', minimum_cohort: 5, measures: [] }, author_id: '1ca7d59b-3160-48b7-99b7-aad2cb648d64', created_at: '2027-06-20T09:00:00Z', approved_at: '2027-06-20T09:00:00Z', published_at: '2027-06-20T09:00:00Z', cancelled: false }
}
describe('edition metric comparability', () => {
 it('allows equivalent retained definitions at different question versions and schedules', () => {
  const a = configuration(), b = configuration(); b.version = 3; b.definition.opens_at = '2028-06-21T09:00:00Z'
  expect(equivalentQuestion(a,b,question)).toBe(true)
  expect(canonical({b:1,a:{y:2,x:3}})).toBe(canonical({a:{x:3,y:2},b:1}))
 })
 it.each(['label','choices','required','help','identity','audience'])('rejects changed %s despite equal numeric versions', change => {
  const a = configuration(), b = configuration(), field = b.definition.form.fields[0]
  if(change==='label')field.label='Something else?'
  if(change==='choices')field.choices=['yes','no','maybe']
  if(change==='required')field.required=false
  if(change==='help')field.help='Use independently observed evidence.'
  if(change==='identity')field.id='3d69935f-f2c5-4f74-b53b-a9d95a7191ca'
  if(change==='audience')b.definition.form.audience='mentor'
  expect(equivalentQuestion(a,b,question)).toBe(false)
 })
 it('includes conditional parent semantics, not only the visible child label', () => {
  const a=configuration(), b=configuration(), child='4d69935f-f2c5-4f74-b53b-a9d95a7191ca'
  for(const value of[a,b])value.definition.form.fields.push({id:child,label:'Explain',type:'text',required:false,help:'',choices:[],visible_when:{field_id:question,equals:'yes'}})
  b.definition.form.fields[0].choices=['yes','no','pending']
  expect(equivalentQuestion(a,b,child)).toBe(false)
 })
 it('requires real exact native measure snapshots even when event-scoped IDs/versions coincide', () => {
  const a=configuration(), b=configuration()
  for(const value of[a,b])value.definition.measures=[{question_id:question,construct:'self_reported_learning',measure:{objective:{id:'122700b0-acf4-4e13-ae54-d339af614225',version:1},measure_id:question}}]
  const first={unit:'respondent',method:'self-reported learning',baseline:null}, changed={...first,unit:'independently evidenced adoption'}
  expect(equivalentQuestion(a,b,question)).toBe(false)
  expect(equivalentQuestion(a,b,question,first,first)).toBe(true)
  expect(equivalentQuestion(a,b,question,first,changed)).toBe(false)
  b.definition.measures[0].measure.objective.version=2
  expect(equivalentQuestion(a,b,question,first,first)).toBe(false)
 })
})
