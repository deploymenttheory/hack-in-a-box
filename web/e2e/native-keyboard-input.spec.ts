import { test, expect } from '@playwright/test'
import { activate, typeInto, inspectCurrent } from '../qualification-native/keyboard'

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
