import { expect, test, type Page } from '@playwright/test'
import type { components } from '../src/api/schema.gen'
import { firstID, person } from './fixtures'

const outcomeID = '71111111-1111-4111-8111-111111111111'
type View = components['schemas']['OutcomeView']
function owned(id = outcomeID): View { return { id, version: 1, current_version: 1, command_id: crypto.randomUUID(), author_id: person.id, actor_id: person.id, at: '2026-10-01T12:00:00Z', reason: 'Deliberate native ownership', definition: { kind: 'pitch', problem: 'PRIVATE idea <script>window.outcomeLeak=true</script>', audience: 'People who use the service', proposed_value: 'Estimated benefit, not an achieved result', solution: '', backlog_url: '', theme_sensitive: false }, provenance: { event_id: firstID, team_id: null }, status: 'accepted', owner_id: person.id, review_at: null, measures: [], evidence_added_id: null, capability: { can_revise: false, can_moderate: false, can_follow_up: true, can_adopt: false, archived: true } } }
async function mock(page: Page, initial = false) {
  const state = { records: new Map<string, View[]>(), requests: [] as { id: string; body: components['schemas']['OutcomeCommand']; expected: string }[], mode: '' as '' | 'after' | 'before' | 'validation', deny: false, offlineRead: false, eventReads: 0, evidence: [] as components['schemas']['OutcomeEvidenceRecord'][] }
  if (initial) state.records.set(outcomeID, [owned()])
  const gates: { preflight?: () => Promise<void>; response?: () => Promise<void>; receipt?: () => Promise<void>; evidence?: () => Promise<void>; history?: () => Promise<void> } = {}
  await page.route('**/api/v1/**', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname
    const fail = (status: number) => route.fulfill({ status, json: { error: { code: status === 403 ? 'forbidden' : status === 409 ? 'revision_conflict' : 'invalid_request', message: 'Synthetic current resource policy', request_id: 'synthetic' } } })
    if (path === '/api/v1/session') return route.fulfill({ json: { person: { ...person, platform_admin: false, display_name: 'Ordinary Member' }, draft_scope: '22222222-2222-4222-8222-222222222222', csrf_token: 'synthetic-csrf-token-32-characters', can_create_events: false } })
    if (path === '/api/v1/inbox') return route.fulfill({ json: { items: [], unread: 0, next_cursor: null } })
    if (path === '/api/v1/events') { state.eventReads++; return route.fulfill({ json: { items: [], next_cursor: null } }) }
    if (path.startsWith('/api/v1/events/')) { state.eventReads++; return fail(403) }
    if (path === '/api/v1/outcomes') return route.fulfill({ json: { items: [...state.records.values()].map(rows => rows.at(-1)), next_cursor: null } })
    if (path === '/api/v1/projects') return route.fulfill({ json: { items: [], next_cursor: null } })
    if (path.startsWith('/api/v1/projects/')) return fail(404)
    const match = path.match(/^\/api\/v1\/outcomes\/([^/]+)(?:\/(.*))?$/)
    if (!match) return fail(404)
    const [, id, suffix] = match, rows = state.records.get(id) ?? [], current = rows.at(-1)
    if (state.deny) return fail(403)
    if (req.method() === 'GET') {
      if (state.offlineRead) return fail(500)
      if (suffix === 'reviews') return route.fulfill({ json: { items: [] } })
      if (suffix === 'evidence') { await gates.evidence?.(); return route.fulfill({ json: { items: state.evidence, next_cursor: null } }) }
      if (suffix === 'history') { await gates.history?.(); return route.fulfill({ json: { items: [...rows].reverse(), next_cursor: null } }) }
      const command = url.searchParams.get('command_id')
      if (command) { await gates.receipt?.(); const receipt = rows.find(row => row.command_id === command); return receipt ? route.fulfill({ json: { ...receipt, current_version: current!.version } }) : fail(404) }
      await gates.preflight?.()
      return current ? route.fulfill({ json: { ...current, current_version: current.version } }) : fail(404)
    }
    const body = req.postDataJSON() as components['schemas']['OutcomeCommand'], expected = req.headers()['if-match']
    state.requests.push({ id, body: structuredClone(body), expected })
    if (state.mode === 'validation') return fail(400)
    if (expected !== `"${current?.version ?? 0}"`) return fail(409)
    if (state.mode === 'before') { state.mode = ''; return route.abort() }
    const next: View = { ...(current ?? owned(id)), id, version: (current?.version ?? 0) + 1, current_version: (current?.version ?? 0) + 1, command_id: body.id, reason: body.reason, definition: body.definition ?? current?.definition ?? owned(id).definition }
    if (body.action === 'propose') { next.status = 'proposed'; next.owner_id = null; next.provenance = { event_id: null, team_id: null }; next.capability = { can_revise: true, can_moderate: false, can_adopt: false, can_follow_up: false, archived: false } }
    if (body.action === 'progress') next.status = body.status as View['status']
    if (body.evidence) state.evidence.push({ ...body.evidence, actor_id: person.id, at: '2026-10-01T12:00:00Z', outcome_version: next.version })
    rows.push(next); state.records.set(id, rows)
    if (state.mode === 'after') { state.mode = ''; return route.abort() }
    await gates.response?.(); return route.fulfill({ json: next })
  })
  return { state, gates }
}
async function propose(page: Page) { await page.goto('/ideas'); await page.getByRole('button', { name: 'Propose an idea', exact: true }).click(); await page.getByLabel('Problem or opportunity', { exact: true }).fill('PRIVATE NEW PITCH <script>window.outcomeLeak=true</script>'); await page.getByLabel('Who benefits?').fill('Organisation colleagues'); await page.getByLabel('Proposed value or estimate (not achieved results)').fill('A proposed benefit'); await page.getByLabel('Why submit this idea?').fill('Review this future idea'); await page.getByRole('button', { name: 'Review proposed idea', exact: true }).click() }

test('ordinary member without event access submits and reads their idea using keyboard/mobile without private storage', async ({ page }, info) => {
  const { state } = await mock(page); await propose(page)
  await page.getByRole('button', { name: 'Submit idea for review' }).focus(); await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Idea and follow-through', exact: true })).toBeVisible()
  expect(state.requests).toHaveLength(1); expect(state.eventReads).toBe(0); expect(state.requests[0].body.provenance).toBeUndefined()
  expect(await page.evaluate(() => [...Object.values(localStorage), ...Object.values(sessionStorage)].join('').includes('PRIVATE NEW PITCH'))).toBe(false)
  expect(await page.evaluate(() => 'outcomeLeak' in window)).toBe(false)
  await expect(page.getByRole('heading', { name: 'Reasoned review status' })).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('ordinary-member-idea.png'), fullPage: true }); expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width)
})
for (const mode of ['after', 'before'] as const) test(`unknown ${mode} save uses exact read-only receipt and only deliberate same-command retry`, async ({ page }) => {
  const { state } = await mock(page); await propose(page); state.mode = mode
  await page.getByRole('button', { name: 'Submit idea for review' }).click(); await expect(page.getByRole('status', { name: 'Outcome save state' })).toContainText('Save outcome is unknown')
  expect(state.requests).toHaveLength(1); await page.getByRole('button', { name: 'Check exact save receipt' }).click()
  if (mode === 'before') { await expect(page.getByRole('status', { name: 'Outcome save state' })).toContainText('original version is still current'); expect(state.requests).toHaveLength(1); await page.getByRole('button', { name: 'Retry the same command explicitly' }).click(); await expect.poll(() => state.requests.length).toBe(2); expect(state.requests[1]).toEqual(state.requests[0]) }
  await expect(page.getByRole('heading', { name: 'Idea and follow-through', exact: true })).toBeVisible(); expect(state.requests).toHaveLength(mode === 'after' ? 1 : 2)
})
test('explicit validation failure preserves unsent input without a false ambiguous or saved receipt', async ({ page }) => {
  const { state } = await mock(page); await propose(page); state.mode = 'validation'; await page.getByRole('button', { name: 'Submit idea for review' }).click()
  await expect(page.getByRole('status', { name: 'Outcome save state' })).toContainText('server rejected this input'); await page.getByRole('button', { name: 'Correct details', exact: true }).click(); await expect(page.getByLabel('Problem or opportunity', { exact: true })).toHaveValue(/PRIVATE NEW PITCH/); await expect(page.getByRole('button', { name: 'Check exact save receipt' })).toHaveCount(0)
})
test('archived ordinary owner records self-report with explicit zero versus unknown, preserving reasoned history', async ({ page }) => {
  const { state } = await mock(page, true); await page.goto(`/ideas/${outcomeID}`)
  await page.getByLabel('What kind of evidence is this?').selectOption('self_report'); await page.getByLabel('Claim or observation').fill('Self-reported working use'); await page.getByLabel('Value (optional exact decimal; blank means unknown)').fill('0')
  const form = page.locator('section.fleet-outcome-action').filter({ has: page.getByRole('heading', { name: 'Restricted evidence record', exact: true }) }); await form.getByLabel('Reason for this change').fill('Separate self-report from demonstrated adoption'); await form.getByRole('button', { name: 'Review restricted evidence record', exact: true }).click(); await form.getByRole('button', { name: 'Save restricted evidence record', exact: true }).click()
  await expect(form.getByRole('status', { name: 'Outcome save state' })).toContainText('Saved on the server'); expect(state.requests[0].body.evidence).toMatchObject({ basis: 'self_report', value: '0', denominator: null, observed_at: null })
  await page.getByRole('button', { name: 'Show restricted evidence' }).click(); await expect(page.getByText('Value: 0 · Denominator: Unknown')).toBeVisible(); await page.getByRole('button', { name: 'Show idea history' }).click(); await expect(page.getByText('Separate self-report from demonstrated adoption', { exact: true })).toBeVisible()
})
test('bounded active resource revocation clears private editors without focus change or automatic write', async ({ page }) => {
  const { state } = await mock(page, true); await page.goto(`/ideas/${outcomeID}`); await page.getByLabel('Claim or observation').fill('REVOKED PRIVATE INPUT'); state.deny = true
  await expect(page.getByRole('heading', { name: 'Idea unavailable', exact: true })).toBeVisible({ timeout: 20_000 }); await expect(page.getByLabel('Claim or observation')).toHaveCount(0); expect(state.requests).toHaveLength(0)
})

test('transient current read failure keeps same-page private input while disabling writes until checked', async ({ page }) => {
  const { state } = await mock(page, true); await page.goto(`/ideas/${outcomeID}`); await page.getByLabel('Claim or observation').fill('PRIVATE UNSENT THROUGH NETWORK FAILURE')
  state.offlineRead = true; await page.getByRole('button', { name: 'Refresh current outcome' }).click(); await expect(page.getByText('Current authority could not be refreshed.', { exact: false })).toBeVisible(); await expect(page.getByLabel('Claim or observation')).toHaveValue('PRIVATE UNSENT THROUGH NETWORK FAILURE'); await expect(page.getByLabel('Claim or observation')).toBeDisabled()
  state.offlineRead = false; await page.getByRole('button', { name: 'Refresh current outcome' }).click(); await expect(page.getByLabel('Claim or observation')).toBeEnabled(); await expect(page.getByLabel('Claim or observation')).toHaveValue('PRIVATE UNSENT THROUGH NETWORK FAILURE'); expect(state.requests).toHaveLength(0)
})
test('leaving during an in-flight current-resource preflight cannot issue a late write', async ({ page }) => {
  const { state, gates } = await mock(page, true); await page.goto(`/ideas/${outcomeID}`); await page.getByLabel('Claim or observation').fill('PRIVATE DEPARTED INTENT')
  const form = page.locator('section.fleet-outcome-action').filter({ has: page.getByRole('heading', { name: 'Restricted evidence record', exact: true }) }); await form.getByLabel('Reason for this change').fill('Deliberate source check')
  let release!: () => void, entered!: () => void; const held = new Promise<void>(resolve => { release = resolve }), reached = new Promise<void>(resolve => { entered = resolve }); gates.preflight = async () => { entered(); await held }
  await form.getByRole('button', { name: 'Review restricted evidence record', exact: true }).click(); await form.getByRole('button', { name: 'Save restricted evidence record', exact: true }).click(); await reached
  page.once('dialog', dialog => dialog.accept()); await page.getByRole('link', { name: 'Check separately published project' }).click(); await expect(page.getByRole('link', { name: 'Reviewed projects' })).toBeVisible(); release(); await page.waitForLoadState('networkidle'); expect(state.requests).toHaveLength(0); await expect(page.getByLabel('Claim or observation')).toHaveCount(0)
})
test('accepted closure with lost response remains explicitly reconcilable after current state refresh', async ({ page }) => {
  const { state } = await mock(page, true); await page.goto(`/ideas/${outcomeID}`)
  const form = page.locator('section.fleet-outcome-action').filter({ has: page.getByRole('heading', { name: 'Owner progress', exact: true }) }); await form.getByLabel('Follow-through status').selectOption('closed'); await form.getByLabel('Reason for this change').fill('Completed follow-through'); state.mode = 'after'; await form.getByRole('button', { name: 'Review owner progress', exact: true }).click(); await form.getByRole('button', { name: 'Save owner progress', exact: true }).click(); await expect(form.getByRole('status', { name: 'Outcome save state' })).toContainText('Save outcome is unknown')
  await page.getByRole('button', { name: 'Refresh current outcome' }).click(); await expect(page.getByText('Reasoned status: closed · Version 2')).toBeVisible(); await form.getByRole('button', { name: 'Check exact save receipt' }).click(); await expect(form.getByRole('status', { name: 'Outcome save state' })).toContainText('exact command was saved'); expect(state.requests).toHaveLength(1)
})
test('published projects search and paginate only positive reviewed fields and honour current revocation',async({page})=>{
 const {state}=await mock(page,true)
 const next='72222222-2222-4222-8222-222222222222',queries:string[]=[]
 const project:components['schemas']['OutcomeProject']={id:outcomeID,version:2,problem:'Reviewed adoption project',solution:'Reviewed solution <script>window.projectLeak=true</script>',links:[{label:'Approved documentation',url:'https://example.test/docs',media:false}],source:{event_id:firstID,team_id:null},event_name:null,team_name:null,follow_up_status:'accepted',published_at:'2026-10-01T12:00:00Z'}
 let deny=false
 await page.route('**/api/v1/projects**',async route=>{
  const url=new URL(route.request().url());queries.push(url.search)
  if(deny)return route.fulfill({status:403,json:{error:{code:'forbidden',message:'Current publication revoked',request_id:'synthetic'}}})
  if(url.pathname===`/api/v1/projects/${outcomeID}`)return route.fulfill({json:project})
  if(url.searchParams.get('search')==='private original')return route.fulfill({json:{items:[],next_cursor:null}})
  return route.fulfill({json:{items:[url.searchParams.has('cursor')?{...project,id:next,problem:'Another reviewed project'}:project],next_cursor:url.searchParams.has('cursor')?null:outcomeID}})
 })
 await page.goto('/projects');await expect(page.getByRole('link',{name:'Reviewed adoption project',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'More reviewed projects'}).click();await expect(page.getByRole('link',{name:'Another reviewed project',exact:true})).toBeVisible()
 expect(queries.some(q=>q.includes('cursor='+outcomeID))).toBe(true)
 await page.getByRole('searchbox', { name: 'Search reviewed problem and solution' }).fill('private original');await page.getByRole('button',{name:'Search',exact:true}).click()
 await expect(page.getByText('No permitted reviewed projects match this search.')).toBeVisible()
 await page.goto(`/projects/${outcomeID}`);await expect(page.getByRole('heading',{name:'Reviewed adoption project',exact:true})).toBeVisible()
 await expect(page.getByRole('link',{name:'Approved documentation'})).toHaveAttribute('rel','noopener noreferrer')
 expect(await page.evaluate(()=>'projectLeak'in window)).toBe(false);expect(state.eventReads).toBe(0)
 deny=true
 await expect(page.getByRole('heading',{name:'Reviewed adoption project',exact:true})).toHaveCount(0,{timeout:20_000})
 await expect(page.getByText('You do not have access to this resource.')).toBeVisible()
})
test('ordinary no-event-role owner opens a native inbox review destination without triggering a write or event lookup',async({page})=>{
 const {state}=await mock(page,true),reviewID='73333333-3333-4333-8333-333333333333'
 await page.route(`**/api/v1/outcome-reviews/${reviewID}`,route=>route.fulfill({json:{id:reviewID,outcome_id:outcomeID,version:2,current_version:2,command_id:crypto.randomUUID(),source_version:1,owner_id:person.id,schedule:{days:30,wall_time:'2026-12-01T12:00:00',timezone:'Europe/London',dst_choice:'',channels:['in_app']},message:{subject:'',body:''},due_at:'2026-12-01T12:00:00Z',state:'approved',effective_state:'approved',reason:'',actor_id:'00000000-0000-0000-0000-000000000000',at:'2026-10-01T12:00:00Z',blockers:[]}}))
 await page.goto(`/ideas/reviews/${reviewID}`);await expect(page.getByRole('heading',{name:'30-day checkpoint'})).toBeVisible()
 await expect(page.getByText('No action or completion is submitted automatically by opening this reminder.')).toBeVisible()
 await page.getByRole('link',{name:'Open your current follow-through'}).click();await expect(page.getByRole('heading',{name:'Idea and follow-through',exact:true})).toBeVisible()
 expect(state.requests).toHaveLength(0);expect(state.eventReads).toBe(0)
})
test('current source-team denial clears event-context choices and unsent team input without blocking general ideas',async({page})=>{
 const {state}=await mock(page)
 const teamID='74444444-4444-4444-8444-444444444444'
 let revoked=false
 await page.route(`**/api/v1/events/${firstID}/teams`,route=>revoked?route.fulfill({status:403,json:{error:{code:'forbidden',message:'Current source team denied',request_id:'synthetic'}}}):route.fulfill({json:{teams:[{id:teamID,name:'PRIVATE SOURCE TEAM',state:'published'}],people:[],requests:[],revision:1,can_manage:false,can_create:false,frozen:true,freeze_at:'2026-09-01T12:00:00Z'}}))
 await page.goto(`/ideas?event=${firstID}`);await page.getByRole('button',{name:'Propose an idea',exact:true}).click()
 await page.getByLabel('Source',{exact:true}).selectOption(teamID)
 await page.getByLabel('Problem or opportunity',{exact:true}).fill('PRIVATE UNSENT TEAM OUTPUT')
 revoked=true
 await expect(page.getByLabel('Problem or opportunity',{exact:true})).toHaveCount(0,{timeout:20_000})
 await expect(page.getByText('PRIVATE SOURCE TEAM',{exact:false})).toHaveCount(0)
 expect(state.requests).toHaveLength(0)
 await page.goto('/ideas');await page.getByRole('button',{name:'Propose an idea',exact:true}).click()
 await expect(page.getByLabel('Problem or opportunity',{exact:true})).toHaveValue('')
})


test('idea directory uses scoped Fleet records and explicit keyboard navigation without publishing private data', async ({ page }) => {
  const { state } = await mock(page, true)
  await page.goto('/ideas')
  await expect(page.getByRole('heading', { name: 'Ideas', exact: true })).toBeVisible()
  await expect(page.getByRole('status').filter({ hasText: '1 loaded' })).toBeVisible()
  await expect(page.getByText('A review status records a deliberate decision.', { exact: false })).toBeVisible()
  const record = page.getByRole('link', { name: 'PRIVATE idea <script>window.outcomeLeak=true</script>', exact: true })
  await expect(record).toBeVisible()
  await page.emulateMedia({ media: 'print' })
  await expect(record).toBeHidden()
  await page.emulateMedia({ media: 'screen' })
  const open = page.getByRole('link', { name: 'Open PRIVATE idea <script>window.outcomeLeak=true</script>', exact: true })
  await open.focus(); await page.keyboard.press('Enter')
  await expect(page).toHaveURL(`/ideas/${outcomeID}`)
  await expect(page.getByRole('heading', { name: 'Idea and follow-through', exact: true })).toBeVisible()
  expect(state.requests).toHaveLength(0)
  expect(state.eventReads).toBe(0)
  expect(await page.evaluate(() => 'outcomeLeak' in window)).toBe(false)
})

for (const outcome of ['pending', 'unknown'] as const) test(`original ${outcome} effect blocks replacement across project and idea routes until its exact receipt`, async ({ page }) => {
  const { state, gates } = await mock(page, true)
  await page.goto(`/ideas/${outcomeID}`)
  const followThrough = () => page.locator('section.fleet-outcome-action').filter({ has: page.getByRole('heading', { name: 'Owner progress', exact: true }) })
  await page.getByLabel('Follow-through status').selectOption('in_progress')
  await followThrough().getByLabel('Reason for this change').fill('PRIVATE ORIGINAL FOLLOW-UP REASON')
  let release = () => {}, reached = () => {}
  const held = new Promise<void>(resolve => { release = resolve }), entered = new Promise<void>(resolve => { reached = resolve })
  if (outcome === 'pending') gates.response = async () => { reached(); await held }
  else state.mode = 'after'
  await followThrough().getByRole('button', { name: 'Review owner progress', exact: true }).click(); await followThrough().getByRole('button', { name: 'Save owner progress', exact: true }).click()
  if (outcome === 'pending') await entered
  else await expect(followThrough().getByRole('status', { name: 'Outcome save state' })).toContainText('Save outcome is unknown')
  expect(state.requests).toHaveLength(1)
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('link', { name: 'Check separately published project' }).click()
  await page.getByRole('link', { name: 'Reviewed projects', exact: true }).click()
  await page.getByRole('link', { name: 'Your ideas', exact: true }).click()
  await page.getByRole('link', { name: /^Open PRIVATE idea/ }).click()
  await expect(followThrough().getByLabel('Reason for this change')).toHaveValue('')
  await expect(followThrough().getByLabel('Reason for this change')).toBeDisabled()
  await expect(followThrough().getByRole('button', { name: 'Review owner progress' })).toBeDisabled()
  await expect(page.getByRole('complementary', { name: 'Original outcome recovery' }).getByText(outcome === 'pending' ? 'Awaiting original acknowledgement' : 'Original outcome unknown', { exact: false })).toBeVisible()
  expect(await page.evaluate(() => [...Object.values(localStorage), ...Object.values(sessionStorage)].join(''))).not.toContain('PRIVATE ORIGINAL FOLLOW-UP REASON')
  if (outcome === 'pending') {
    release()
    await expect(followThrough().getByLabel('Reason for this change')).toBeEnabled()
  } else {
    await page.getByRole('button', { name: 'Check earlier exact save receipt' }).click()
    await expect(page.getByRole('status', { name: 'Earlier command recovery state' })).toContainText('exact earlier command was saved')
    await expect(followThrough().getByLabel('Reason for this change')).toBeEnabled()
  }
  expect(state.requests).toHaveLength(1)
})

test('private outcome direct remount waits for an actual current read and current denial latches through partial restoration', async ({ page }) => {
  const { state, gates } = await mock(page, true)
  await page.goto(`/ideas/${outcomeID}`)
  await expect(page.getByRole('heading', { name: /^PRIVATE idea/ })).toBeVisible()
  await page.getByRole('link', { name: 'Check separately published project' }).click()
  await page.getByRole('link', { name: 'Reviewed projects', exact: true }).click()
  await page.getByRole('link', { name: 'Your ideas', exact: true }).click()
  let release = () => {}, reached = () => {}
  const held = new Promise<void>(resolve => { release = resolve }), entered = new Promise<void>(resolve => { reached = resolve })
  gates.preflight = async () => { reached(); await held }
  await page.getByRole('link', { name: /^Open PRIVATE idea/ }).click()
  await entered
  await expect(page.getByRole('heading', { name: /^PRIVATE idea/ })).toHaveCount(0)
  release(); gates.preflight = undefined
  await expect(page.getByRole('heading', { name: /^PRIVATE idea/ })).toBeVisible()
  state.deny = true
  await page.getByRole('button', { name: 'Refresh current outcome' }).click()
  await expect(page.getByRole('heading', { name: 'Idea unavailable', exact: true })).toBeVisible()
  state.deny = false
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  await expect(page.getByRole('heading', { name: /^PRIVATE idea/ })).toHaveCount(0)
  await page.getByRole('button', { name: 'Check current outcome access' }).click()
  await expect(page.getByRole('heading', { name: /^PRIVATE idea/ })).toBeVisible()
  await page.emulateMedia({ media: 'print' })
  await expect(page.getByRole('heading', { name: /^PRIVATE idea/ })).toBeHidden()
  expect(state.requests).toHaveLength(0)
})

test('a rejected exact retry does not settle the original unknown command across a new proposal editor', async ({ page }) => {
  const { state } = await mock(page)
  await propose(page); state.mode = 'before'
  await page.getByRole('button', { name: 'Submit idea for review', exact: true }).click()
  await expect(page.getByRole('status', { name: 'Outcome save state' })).toContainText('Save outcome is unknown')
  await page.getByRole('button', { name: 'Check exact save receipt' }).click()
  await expect(page.getByRole('button', { name: 'Retry the same command explicitly' })).toBeVisible()
  state.mode = 'validation'
  await page.getByRole('button', { name: 'Retry the same command explicitly' }).click()
  await expect(page.getByRole('status', { name: 'Outcome save state' })).toContainText('original outcome remains unknown')
  expect(state.requests).toHaveLength(2); expect(state.requests[1]).toEqual(state.requests[0])
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('link', { name: 'Browse reviewed projects' }).click()
  await page.getByRole('link', { name: 'Your ideas', exact: true }).click()
  await page.getByRole('button', { name: 'Propose an idea', exact: true }).click()
  await expect(page.getByLabel('Problem or opportunity', { exact: true })).toBeDisabled()
  await expect(page.getByLabel('Problem or opportunity', { exact: true })).toHaveValue('')
  await expect(page.getByRole('button', { name: 'Check earlier exact save receipt' })).toBeVisible()
  expect(state.requests).toHaveLength(2)
})

test('matched synthetic outcome directory and source screenshots', async ({ page }, info) => {
 const {state}=await mock(page,true)
 await page.goto('/ideas');await expect(page.getByRole('heading',{name:'Ideas',exact:true})).toBeVisible()
 await expect(page.getByRole('link',{name:'PRIVATE idea <script>window.outcomeLeak=true</script>',exact:true})).toBeVisible()
 await page.screenshot({path:info.outputPath('outcome-directory.png')})
 await page.goto(`/ideas/${outcomeID}`);await expect(page.getByRole('heading',{name:'Idea and follow-through',exact:true})).toBeVisible()
 await expect(page.getByRole('heading',{name:'PRIVATE idea <script>window.outcomeLeak=true</script>',exact:true})).toBeVisible()
 await page.screenshot({path:info.outputPath('outcome-source.png')})
 expect(state.requests).toHaveLength(0)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width)
})

for(const projection of ['evidence','history'] as const)test(`reopening restricted ${projection} cannot restore cached private records while its current read is held`,async({page})=>{
 const {state,gates}=await mock(page,true)
 const canary=`PRIVATE ORIGINAL ${projection.toUpperCase()} RECORD`
 if(projection==='evidence')state.evidence.push({id:crypto.randomUUID(),basis:'self_report',claim:canary,value:'0',denominator:null,source:'Synthetic source',observed_at:null,measures:[],outcome_version:1,actor_id:person.id,at:'2026-10-01T12:00:00Z'})
 else state.records.get(outcomeID)![0].reason=canary
 await page.goto(`/ideas/${outcomeID}`)
 const label=projection==='evidence'?'restricted evidence':'idea history'
 await page.getByRole('button',{name:`Show ${label}`,exact:true}).click()
 await expect(page.getByText(canary,{exact:true})).toBeVisible()
 await page.getByRole('button',{name:`Hide ${label}`,exact:true}).click()
 let release=()=>{},reached=()=>{}
 const held=new Promise<void>(resolve=>{release=resolve}),entered=new Promise<void>(resolve=>{reached=resolve})
 gates[projection]=async()=>{reached();await held}
 await page.getByRole('button',{name:`Show ${label}`,exact:true}).click();await entered
 await expect(page.getByText(canary,{exact:true})).toHaveCount(0)
 release();gates[projection]=undefined
 await expect(page.getByText(canary,{exact:true})).toBeVisible()
 expect(state.requests).toHaveLength(0)
})

test('original unknown outcome recovery remains available after all write capabilities disappear',async({page})=>{
 const {state}=await mock(page,true)
 await page.goto(`/ideas/${outcomeID}`)
 const progress=page.locator('section.fleet-outcome-action').filter({has:page.getByRole('heading',{name:'Owner progress',exact:true})})
 await progress.getByLabel('Reason for this change').fill('PRIVATE EARLIER OWNER REASON')
 await progress.getByRole('button',{name:'Review owner progress',exact:true}).click()
 state.mode='after'
 await progress.getByRole('button',{name:'Save owner progress',exact:true}).click()
 await expect(progress.getByRole('status',{name:'Outcome save state'})).toContainText('Save outcome is unknown')
 const current=state.records.get(outcomeID)!.at(-1)!
 current.capability={...current.capability,can_follow_up:false,can_revise:false,can_moderate:false,can_adopt:false}
 await page.getByRole('button',{name:'Refresh current outcome'}).click()
 await expect(page.getByRole('heading',{name:'Owner progress',exact:true})).toHaveCount(0)
 await expect(page.getByRole('heading',{name:'Restricted evidence record',exact:true})).toHaveCount(0)
 await expect(page.getByRole('complementary',{name:'Original outcome recovery'})).toContainText('Original outcome unknown')
 await page.getByRole('button',{name:'Check earlier exact save receipt',exact:true}).click()
 await expect(page.getByRole('status',{name:'Earlier command recovery state'})).toContainText('exact earlier command was saved')
 expect(state.requests).toHaveLength(1)
})

test('native outcome correction returns keyboard focus to the actual mounted initiating control',async({page})=>{
 const {state}=await mock(page,true)
 await page.goto(`/ideas/${outcomeID}`)
 const progress=page.locator('section.fleet-outcome-action').filter({has:page.getByRole('heading',{name:'Owner progress',exact:true})})
 await progress.getByLabel('Reason for this change').fill('Synthetic focus review')
 const opener=progress.getByRole('button',{name:'Review owner progress',exact:true})
 await opener.focus();await page.keyboard.press('Enter')
 await expect(progress.getByRole('heading',{name:'Review owner progress',exact:true})).toBeFocused()
 await progress.getByRole('button',{name:'Correct details',exact:true}).click()
 await expect(opener).toBeFocused()
 expect(state.requests).toHaveLength(0)
})
