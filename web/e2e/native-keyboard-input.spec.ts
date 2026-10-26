import { test, expect } from '@playwright/test'
import { activate, typeInto, inspectCurrent, openLocal, readOriginalAcknowledgement } from '../qualification-native/keyboard'

test('native text-entry helper observes a read-pause rejection before deliberately re-entering the same field', async ({ page }) => {
 await page.setContent('<fieldset><textarea aria-label="Synthetic current evidence">Earlier value</textarea></fieldset>')
 const control = page.getByLabel('Synthetic current evidence'), press = page.keyboard.press.bind(page.keyboard), insert = page.keyboard.insertText.bind(page.keyboard)
 let paused = false, inserts = 0
 page.keyboard.press = async (key, options) => { await press(key, options); if (key === 'ControlOrMeta+A' && !paused) { paused = true; await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = true }) } }
 page.keyboard.insertText = async text => { inserts++; await insert(text); await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = false }) }
 await typeInto(control, 'Deliberately entered full evidence')
 await expect(control).toHaveValue('Deliberately entered full evidence')
 expect(inserts).toBe(2)
})
test('native checkbox helper observes a read-pause rejection before deliberately toggling the same checkbox', async ({ page }) => {
 await page.setContent('<fieldset><label><input type="checkbox" checked>Pending observation</label></fieldset>')
 const control = page.getByRole('checkbox', { name: 'Pending observation' }), press = page.keyboard.press.bind(page.keyboard)
 let spaces = 0
 page.keyboard.press = async (key, options) => {
  if (key === 'Space') { spaces++; if (spaces === 1) await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = true }) }
  await press(key, options)
  if (key === 'Space') await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = false })
 }
 await activate(control, 'Space')
 await expect(control).not.toBeChecked()
 expect(spaces).toBe(2)
})

test('an unobserved failed input is not permission to repeat text entry', async ({ page }) => {
 await page.setContent('<textarea aria-label="Synthetic current evidence">Earlier value</textarea>')
 let inserts = 0
 page.keyboard.insertText = async () => { inserts++ }
 await expect(typeInto(page.getByLabel('Synthetic current evidence'), 'New observation')).rejects.toThrow('Native input did not accept its trusted keys')
 expect(inserts).toBe(1)
})
test('a paused replacement field cannot receive the old text entry', async ({ page }) => {
 await page.setContent('<fieldset><textarea aria-label="Synthetic current evidence">Earlier value</textarea></fieldset>')
 const control = page.getByLabel('Synthetic current evidence'), press = page.keyboard.press.bind(page.keyboard), insert = page.keyboard.insertText.bind(page.keyboard)
 let inserts = 0
 page.keyboard.press = async (key, options) => { await press(key, options); if (key === 'ControlOrMeta+A') await control.evaluate(element => { element.closest('fieldset')!.disabled = true; element.replaceWith(element.cloneNode(true)) }) }
 page.keyboard.insertText = async text => { inserts++; await insert(text); await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = false }) }
 await expect(typeInto(control, 'Old private intended observation')).rejects.toThrow('Current native input was replaced')
 await expect(control).toHaveValue('Earlier value')
 expect(inserts).toBe(1)
})
test('command-button activation never uses local input re-entry', async ({ page }) => {
 await page.setContent('<fieldset><button>Confirm a command</button></fieldset>')
 const press = page.keyboard.press.bind(page.keyboard)
 let enters = 0
 page.keyboard.press = async (key, options) => { if (key === 'Enter') { enters++; await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = true }) } await press(key, options); if (key === 'Enter') await page.locator('fieldset').evaluate(fieldset => { (fieldset as HTMLFieldSetElement).disabled = false }) }
 await activate(page.getByRole('button', { name: 'Confirm a command' }))
 expect(enters).toBe(1)
})

test('a local inspector opening rejected during a current-read pause is not a completed navigation', async ({ page }) => {
 await page.setContent('<button data-record-focus="current-team" aria-disabled="false">Inspect current team</button><h2 tabindex="-1" hidden>Current team inspector</h2><output></output><script>const button=document.querySelector("button");button.onclick=event=>{if(button.getAttribute("aria-disabled")==="true"){event.preventDefault();return;}document.querySelector("output").textContent="Current team inspector";const heading=document.querySelector("h2");heading.hidden=false;heading.focus()}</script>')
 const press=page.keyboard.press.bind(page.keyboard),button=page.getByRole('button',{name:'Inspect current team'})
 let enters=0
 page.keyboard.press=async(key,options)=>{
  if(key==='Enter'){enters++;if(enters===1)await button.evaluate(element=>element.setAttribute('aria-disabled','true'))}
  await press(key,options)
  if(key==='Enter')await button.evaluate(element=>element.setAttribute('aria-disabled','false'))
 }
 await inspectCurrent(button,page.getByRole('heading',{name:'Current team inspector'}))
 expect(enters).toBe(2)
 expect(await page.locator('output').textContent()).toBe('Current team inspector')
})

test('an accepted local inspection is never repeated even if its key observed a read pause',async({page})=>{
 await page.setContent('<button data-record-focus="current-team">Inspect current team</button><h2 tabindex="-1" hidden>Current team inspector</h2><script>document.querySelector("button").onclick=()=>{const heading=document.querySelector("h2");heading.hidden=false;heading.focus()}</script>')
 const button=page.getByRole('button',{name:'Inspect current team'}),press=page.keyboard.press.bind(page.keyboard)
 let enters=0
 page.keyboard.press=async(key,options)=>{if(key==='Enter'){enters++;await button.evaluate(element=>element.setAttribute('aria-disabled','true'))}await press(key,options)}
 await inspectCurrent(button,page.getByRole('heading',{name:'Current team inspector'}))
 expect(enters).toBe(1)
})
test('an unobserved prevented local activation is not permission to repeat navigation',async({page})=>{
 await page.setContent('<button data-record-focus="current-team" onclick="event.preventDefault()">Inspect current team</button>')
 const press=page.keyboard.press.bind(page.keyboard);let enters=0
 page.keyboard.press=async(key,options)=>{if(key==='Enter')enters++;await press(key,options)}
 await expect(inspectCurrent(page.getByRole('button',{name:'Inspect current team'}),page.getByRole('heading'))).rejects.toThrow('not positively rejected')
 expect(enters).toBe(1)
})
test('a replaced paused inspector target cannot receive the earlier local navigation',async({page})=>{
 await page.setContent('<button data-record-focus="current-team" onclick="if(this.getAttribute(\'aria-disabled\')===\'true\')event.preventDefault()">Inspect current team</button>')
 const button=page.getByRole('button',{name:'Inspect current team'}),press=page.keyboard.press.bind(page.keyboard);let enters=0
 page.keyboard.press=async(key,options)=>{
  if(key==='Enter'){enters++;await button.evaluate(element=>element.setAttribute('aria-disabled','true'))}
  await press(key,options)
  if(key==='Enter')await button.evaluate(element=>{const replacement=element.cloneNode(true) as HTMLElement;replacement.removeAttribute('aria-disabled');element.replaceWith(replacement)})
 }
 await expect(inspectCurrent(button,page.getByRole('heading'))).rejects.toThrow('target was replaced')
 expect(enters).toBe(1)
})

// Native Base UI focusableWhenDisabled semantics: the current read rejects
// a trusted key before a local editor opener's callback can run.
test('local final-card review opener survives a positively rejected current-read key without a write',async({page})=>{
 await page.setContent('<button aria-disabled="false">Review saved final card confirmation</button><label hidden>Recorded panel reason<textarea></textarea></label><script>const button=document.querySelector("button");button.onclick=event=>{if(button.getAttribute("aria-disabled")==="true"){event.preventDefault();return;}const label=document.querySelector("label");label.hidden=false;label.querySelector("textarea").focus()}</script>')
 const button=page.getByRole('button',{name:'Review saved final card confirmation'}),press=page.keyboard.press.bind(page.keyboard)
 let enters=0,writes=0
 page.on('request',request=>{if(!['GET','HEAD'].includes(request.method()))writes++})
 page.keyboard.press=async(key,options)=>{if(key==='Enter'){enters++;if(enters===1)await button.evaluate(element=>element.setAttribute('aria-disabled','true'))}await press(key,options);if(key==='Enter')await button.evaluate(element=>element.setAttribute('aria-disabled','false'))}
 await openLocal(button,page.getByLabel('Recorded panel reason'))
 await expect(page.getByLabel('Recorded panel reason')).toBeVisible({timeout:750})
 expect(enters).toBe(2);expect(writes).toBe(0)
})

test('a current-context change refuses to retarget the same paused local opener',async({page})=>{
 await page.setContent('<main data-private-qualification><p>Current team A</p><button aria-disabled="false">Open local review</button></main><script>document.querySelector("button").onclick=event=>{if(event.currentTarget.getAttribute("aria-disabled")==="true")event.preventDefault()}</script>')
 const button=page.getByRole('button',{name:'Open local review'}),press=page.keyboard.press.bind(page.keyboard);let enters=0
 page.keyboard.press=async(key,options)=>{if(key==='Enter'){enters++;await button.evaluate(element=>element.setAttribute('aria-disabled','true'))}await press(key,options);if(key==='Enter')await page.locator('main').evaluate(element=>{element.querySelector('p')!.textContent='Current team B';element.querySelector('button')!.setAttribute('aria-disabled','false')})}
 await expect(openLocal(button,page.getByLabel('Recorded panel reason'))).rejects.toThrow('target was replaced')
 expect(enters).toBe(1)
})
test('a second read-pause rejection cannot loop a local opener',async({page})=>{
 await page.setContent('<button aria-disabled="false" onclick="if(this.getAttribute(\'aria-disabled\')===\'true\')event.preventDefault()">Open local review</button>')
 const button=page.getByRole('button',{name:'Open local review'}),press=page.keyboard.press.bind(page.keyboard);let enters=0
 page.keyboard.press=async(key,options)=>{if(key==='Enter'){enters++;await button.evaluate(element=>element.setAttribute('aria-disabled','true'))}await press(key,options);if(key==='Enter')await button.evaluate(element=>element.setAttribute('aria-disabled','false'))}
 await expect(openLocal(button,page.getByLabel('Recorded panel reason'))).rejects.toThrow('not positively rejected')
 expect(enters).toBe(2)
})
test('an HTTP write is never accepted as a local opening or repeated',async({page})=>{
 await page.route('https://local.example.test/write',route=>route.fulfill({status:200,body:'ok',headers:{'access-control-allow-origin':'*'}}))
 await page.setContent('<button>Open local review</button><h2 tabindex="-1" hidden>Local review</h2><script>document.querySelector("button").onclick=()=>{fetch("https://local.example.test/write",{method:"POST"});const heading=document.querySelector("h2");heading.hidden=false;heading.focus()}</script>')
 const press=page.keyboard.press.bind(page.keyboard);let enters=0
 page.keyboard.press=async(key,options)=>{if(key==='Enter')enters++;await press(key,options)}
 await expect(openLocal(page.getByRole('button',{name:'Open local review'}),page.getByRole('heading',{name:'Local review'}))).rejects.toThrow('must not send an HTTP mutation')
 expect(enters).toBe(1)
})

const originalRecordIDs={actor:'10000000-0000-4000-8000-000000000001',event:'10000000-0000-4000-8000-000000000002',run:'10000000-0000-4000-8000-000000000003',generation:'10000000-0000-4000-8000-000000000004',request:'10000000-0000-4000-8000-000000000005'}
async function declaredOriginalRecord(page:import('@playwright/test').Page){
 const owner=JSON.stringify({actor:originalRecordIDs.actor,event:originalRecordIDs.event,run:originalRecordIDs.run,generation:originalRecordIDs.generation}),record=JSON.stringify({request:originalRecordIDs.request,event_revision:2,panel_version:0})
 let reads=0
 await page.route('https://original.example.test/**',route=>{if(new URL(route.request().url()).pathname==='/receipt'){reads++;return route.fulfill({body:'synthetic receipt'})}return route.fulfill({contentType:'text/html',body:`<aside aria-label="Original panel command history" data-original-panel-owner='${owner}'><p data-original-panel-record='${record}'>Expected event revision2 · Panel version0 · Outcome unknown<button onclick="window.accepted=(window.accepted||0)+1;fetch('/receipt');document.querySelector('h2').hidden=false">Read original panel acknowledgement</button></p></aside><p id="unrelated">Unrelated team label pending</p><h2 hidden>Exact original receipt</h2><button id="other" onclick="fetch('/receipt')">Other current action</button>`})})
 await page.goto(`https://original.example.test/events/${originalRecordIDs.event}/panel/${originalRecordIDs.run}`)
 return {control:page.getByRole('button',{name:'Read original panel acknowledgement',exact:true}),destination:page.getByRole('heading',{name:'Exact original receipt'}),reads:()=>reads}
}
for(const mutation of ['actor','event','run','generation','request','event_revision','panel_version','record-text','record-clone','owner-clone','button-replacement','reparent','url','control-text','href','wrong-target'] as const)test(`declared original record refuses ${mutation} at the trusted key boundary without a receipt read`,async({page})=>{
 const original=await declaredOriginalRecord(page),press=page.keyboard.press.bind(page.keyboard);let enters=0
 page.keyboard.press=async(key,options)=>{
  if(key==='Enter'){
   enters++
   await page.evaluate(kind=>{
    const record=document.querySelector('[data-original-panel-record]')!,owner=document.querySelector('[data-original-panel-owner]')!,button=record.querySelector('button')!
    if(['actor','event','run','generation'].includes(kind)){const identity=JSON.parse(owner.getAttribute('data-original-panel-owner')!);identity[kind]='20000000-0000-4000-8000-000000000001';owner.setAttribute('data-original-panel-owner',JSON.stringify(identity))}
    else if(['request','event_revision','panel_version'].includes(kind)){const identity=JSON.parse(record.getAttribute('data-original-panel-record')!);identity[kind]=kind==='request'?'20000000-0000-4000-8000-000000000001':99;record.setAttribute('data-original-panel-record',JSON.stringify(identity))}
    else if(kind==='record-text')record.prepend('Different original attempt ')
    else if(kind==='record-clone')record.replaceWith(record.cloneNode(true))
    else if(kind==='owner-clone')owner.replaceWith(owner.cloneNode(true))
    else if(kind==='button-replacement')button.replaceWith(button.cloneNode(true))
    else if(kind==='url')history.replaceState(null,'','/events/20000000-0000-4000-8000-000000000001/panel/20000000-0000-4000-8000-000000000002')
    else if(kind==='control-text')button.textContent='Different original request control'
    else if(kind==='href')button.setAttribute('href','/different-original')
    else if(kind==='reparent'){const other=document.createElement('p');other.setAttribute('data-original-panel-record',record.getAttribute('data-original-panel-record')!);owner.append(other);other.append(button)}
    else (document.querySelector('#other') as HTMLButtonElement).focus()
   },mutation)
  }
  await press(key,options)
 }
 await expect(readOriginalAcknowledgement(original.control,original.destination)).rejects.toThrow(/not positively rejected|target was replaced/)
 expect(enters).toBe(1);expect(original.reads()).toBe(0)
})
for(const malformed of ['missing-record','missing-owner','null-marker','invalid-revision'] as const)test(`declared original record has no fallback for ${malformed}`,async({page})=>{
 const original=await declaredOriginalRecord(page),press=page.keyboard.press.bind(page.keyboard);let enters=0
 await page.evaluate(kind=>{const record=document.querySelector('[data-original-panel-record]')!,owner=document.querySelector('[data-original-panel-owner]')!;if(kind==='missing-record')record.removeAttribute('data-original-panel-record');else if(kind==='missing-owner')owner.removeAttribute('data-original-panel-owner');else if(kind==='null-marker')record.setAttribute('data-original-panel-record','null');else record.setAttribute('data-original-panel-record',JSON.stringify({request:'10000000-0000-4000-8000-000000000005',event_revision:-1,panel_version:0}))},malformed)
 page.keyboard.press=async(key,options)=>{if(key==='Enter')enters++;await press(key,options)}
 await expect(readOriginalAcknowledgement(original.control,original.destination)).rejects.toThrow(/declared original panel/)
 expect(enters).toBe(0);expect(original.reads()).toBe(0)
})
test('declared original record allows one accepted receipt read even when completion retires that record',async({page})=>{
 const original=await declaredOriginalRecord(page),press=page.keyboard.press.bind(page.keyboard);let enters=0
 await original.control.evaluate(button=>button.addEventListener('click',()=>{const owner=button.closest('[data-original-panel-owner]')!;owner.setAttribute('data-original-panel-owner','retired after accepted original read')}))
 page.keyboard.press=async(key,options)=>{if(key==='Enter')enters++;await press(key,options)}
 await readOriginalAcknowledgement(original.control,original.destination)
 await expect.poll(original.reads).toBe(1);expect(enters).toBe(1)
})
