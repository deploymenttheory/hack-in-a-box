import {expect,type Locator} from '@playwright/test'

// Actual native focus traversal. Merely pressing a locator can implicitly wake
// a background tab and race the real source-read pause. The journey activates
// pages once when switching actors, then traverses their current enabled focus
// order. Never force focus or mutate DOM.
export async function reach(control:Locator){
 const page=control.page();await expect(control).toBeVisible()
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
 await reach(control)
 const label=await control.locator('option').evaluateAll((options,wanted)=>options.find(option=>option instanceof HTMLOptionElement&&option.value===wanted)?.textContent,value)
 if(!label)throw new Error('Requested native select option is not currently disclosed')
 await control.page().keyboard.type(label);await control.page().keyboard.press('Tab');await expect(control).toHaveValue(value)
}

// Observe actual trusted input acceptance, rather than treating an enabled
// check before a later global key as proof that the key activated its target.
export async function observeActivation(control:Locator){
 await reach(control)
 const page=control.page(),key=`__nativeActivation_${Date.now()}_${Math.random()}`
 await control.evaluate((element,key)=>{
  const state={keys:0,clicks:0,paused:false,focused:false,connected:true}
  const keydown=(event:KeyboardEvent)=>{if(event.isTrusted&&event.key==='Enter'){state.keys++;state.connected=element.isConnected;state.paused=element.matches(':disabled,[aria-disabled="true"]')||!!element.closest('[inert]');state.focused=element===element.ownerDocument.activeElement}}
  const click=(event:MouseEvent)=>{if(event.isTrusted&&event.target instanceof Node&&element.contains(event.target))state.clicks++}
  element.ownerDocument.addEventListener('keydown',keydown,true);element.ownerDocument.addEventListener('click',click,true)
  Object.assign(window,{[key]:{state,dispose:()=>{element.ownerDocument.removeEventListener('keydown',keydown,true);element.ownerDocument.removeEventListener('click',click,true)}}})
 },key)
 try{
  await page.keyboard.press('Enter')
  return await page.evaluate(key=>{const value=(window as unknown as Record<string,{state:{keys:number;clicks:number;paused:boolean;focused:boolean;connected:boolean}}>)[key];return value.state},key)
 }finally{
  await page.evaluate(key=>{const values=window as unknown as Record<string,{dispose:()=>void}>;values[key]?.dispose();delete values[key]},key)
 }
}
