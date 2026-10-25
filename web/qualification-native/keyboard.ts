import {expect,type Locator} from '@playwright/test'

// Actual native focus traversal. Merely pressing a locator can implicitly wake
// a background tab and race the real source-read pause. Activate the page first,
// then traverse its current enabled focus order; never force focus or mutate DOM.
export async function reach(control:Locator){
 const page=control.page();await page.bringToFront();await expect(control).toBeVisible()
 for(let step=0;step<180;step++){
  await expect(control).toBeEnabled()
  await expect.poll(()=>control.evaluate(element=>!element.closest('[inert]'))).toBe(true)
  if(await control.evaluate(element=>element===element.ownerDocument.activeElement))return
  await page.keyboard.press('Tab')
 }
 throw new Error(`Native keyboard focus could not reach ${control.toString()} in180 tabs`)
}
export async function activate(control:Locator,key:'Enter'|'Space'='Enter'){await reach(control);await control.page().keyboard.press(key)}
export async function typeInto(control:Locator,value:string){await reach(control);await control.page().keyboard.press('ControlOrMeta+A');await control.page().keyboard.insertText(value)}
export async function selectNative(control:Locator,value:string){
 const label=await control.locator('option').evaluateAll((options,wanted)=>options.find(option=>option instanceof HTMLOptionElement&&option.value===wanted)?.textContent,value)
 if(!label)throw new Error('Requested native select option is not currently disclosed')
 await reach(control);await control.page().keyboard.type(label);await control.page().keyboard.press('Tab');await expect(control).toHaveValue(value)
}
