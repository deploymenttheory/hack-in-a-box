import {expect,type Locator,type Request} from '@playwright/test'

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
// Recover only a positively observed rejected local input, never a command.
// Native polling/authority checks remain enabled; replacement elements and
// accepted-but-wrong values are genuine failures, not permission to replay.
async function acceptedInput(control:Locator,enter:()=>Promise<void>,accepted:()=>Promise<boolean>){
 await reach(control)
 const original=await control.elementHandle()
 if(!original)throw new Error('Current native input is absent')
 const same=async()=>{
  if(!await original.evaluate(element=>element.isConnected)||!await control.evaluate((element,expected)=>element===expected,original))throw new Error('Current native input was replaced; no input was retargeted')
 }
 try{
  for(let attempt=0;attempt<2;attempt++){
   await same();await reach(control);await same()
   const observation=await original.evaluateHandle(element=>{
    const paused=()=>element.matches(':disabled,[aria-disabled="true"]')||!!element.closest('[inert]')
    const state={paused:paused(),inputs:0}
    const observer=new MutationObserver(()=>{if(paused())state.paused=true})
    observer.observe(element.ownerDocument.documentElement,{subtree:true,attributes:true,attributeFilter:['disabled','aria-disabled','inert']})
    const input=(event:Event)=>{if(event.isTrusted&&event.target===element)state.inputs++}
    element.ownerDocument.addEventListener('input',input,true)
    return {state,dispose:()=>{observer.disconnect();element.ownerDocument.removeEventListener('input',input,true)}}
   })
   let observed:{paused:boolean;inputs:number}
   try{await enter();observed=await observation.evaluate(value=>value.state)}
   finally{await observation.evaluate(value=>value.dispose());await observation.dispose()}
   await same()
   if(await accepted()){await expect(control).toBeEnabled();await same();if(await accepted())return;throw new Error('Accepted native input changed after the current read')}
   if(!observed.paused||observed.inputs!==0||attempt!==0)throw new Error('Native input did not accept its trusted keys; no command was replayed')
   // Only an observed pause with zero accepted input permits one deliberate
   // text/checkbox re-entry after that same current element becomes enabled.
   await expect(control).toBeEnabled();await same()
  }
 }finally{await original.dispose()}
}
export async function activate(control:Locator,key:'Enter'|'Space'='Enter'){
 if(key==='Space'&&await control.evaluate(element=>element instanceof HTMLInputElement&&element.type==='checkbox')){
  const checked=await control.isChecked()
  await acceptedInput(control,()=>control.page().keyboard.press(key),async()=>await control.isChecked()!==checked)
  return
 }
 await reach(control);await control.page().keyboard.press(key)
}
export async function typeInto(control:Locator,value:string){
 await acceptedInput(control,async()=>{await control.page().keyboard.press('ControlOrMeta+A');await control.page().keyboard.insertText(value)},async()=>await control.inputValue()===value)
}
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

// Explicitly opted-in local UI/read-only steps only: editor/review opening,
// inspection and navigation. Mutation confirmation must use its own command
// barrier. A positively rejected key permits one re-entry of the unchanged step.
async function captureOriginalRecord(original:import('@playwright/test').ElementHandle<HTMLElement|SVGElement>){
 return original.evaluateHandle(element=>{
  const record=element.closest('[data-original-panel-record]'),owner=record?.closest('[data-original-panel-owner]')
  if(!record||!owner||owner.getAttribute('aria-label')!=='Original panel command history'||!owner.contains(record)||!record.contains(element))throw new Error('A declared original panel record and owner are required')
  const parse=(value:string|null)=>{try{const parsed:unknown=JSON.parse(value??'');if(!parsed||typeof parsed!=='object'||Array.isArray(parsed))throw new Error();return parsed as Record<string,unknown>}catch{throw new Error('Malformed declared original panel context')}}
  const ownerIdentity=parse(owner.getAttribute('data-original-panel-owner')),requestIdentity=parse(record.getAttribute('data-original-panel-record')),uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
  if(!['actor','event','run','generation'].every(key=>typeof ownerIdentity[key]==='string'&&uuid.test(ownerIdentity[key] as string))||typeof requestIdentity.request!=='string'||!uuid.test(requestIdentity.request)||!['event_revision','panel_version'].every(key=>Number.isSafeInteger(requestIdentity[key])&&(requestIdentity[key] as number)>=0)||location.pathname!==`/events/${ownerIdentity.event}/panel/${ownerIdentity.run}`)throw new Error('Invalid declared original panel identity')
  const read=()=>JSON.stringify({url:location.href,tag:element.tagName,name:element.textContent,href:element.getAttribute('href'),owner:owner.getAttribute('data-original-panel-owner'),ownerName:owner.getAttribute('aria-label'),record:record.getAttribute('data-original-panel-record'),text:record.textContent})
  const identity=read()
  return {read,identity,current:()=>element.isConnected&&record.isConnected&&owner.isConnected&&element.closest('[data-original-panel-record]')===record&&record.closest('[data-original-panel-owner]')===owner&&owner.contains(record)&&record.contains(element)&&read()===identity}
 })
}

async function openLocalStep(control:Locator,destination:Locator,outcome:'focused'|'visible',originalRecord=false){
 await reach(control)
 const original=await control.elementHandle()
 if(!original)throw new Error('Current native inspector target is absent')
 let declared:Awaited<ReturnType<typeof captureOriginalRecord>>|undefined
 try{if(originalRecord)declared=await captureOriginalRecord(original)}catch(error){await original.dispose();throw error}
 const context=()=>declared?declared.evaluate(value=>value.read()):original.evaluate(element=>JSON.stringify({url:location.href,tag:element.tagName,name:element.textContent,record:element.getAttribute('data-record-focus'),href:element.getAttribute('href'),workspace:(element.closest('[data-private-qualification]')??element.ownerDocument.body).textContent}))
 const identity=declared?await declared.evaluate(value=>value.identity):await context()
 const mutations:string[]=[]
 const request=(value:Request)=>{if(!['GET','HEAD','OPTIONS'].includes(value.method()))mutations.push(value.method())}
 control.page().on('request',request)
 const same=async()=>{
  if(!await original.evaluate(element=>element.isConnected)||!await control.evaluate((element,expected)=>element===expected,original)||declared&&!await declared.evaluate(value=>value.current())||await context()!==identity)throw new Error('Current inspector target was replaced; no navigation was retargeted')
 }
 try{
  for(let attempt=0;attempt<2;attempt++){
   await same();await reach(control);await same()
   const observation=await original.evaluateHandle((element,recordScope)=>{
    const paused=()=>element.matches(':disabled,[aria-disabled="true"]')||!!element.closest('[inert]')
    const state={keys:0,rejectedKeys:0,accepted:0,blocked:0,unexpected:0}
    const originalGuard=(event:Event)=>{
     if(!recordScope||!event.isTrusted||event.type==='keydown'&&(event as KeyboardEvent).key!=='Enter')return
     if(!recordScope.current()||!(event.target instanceof Node)||!element.contains(event.target)||event.type==='keydown'&&element!==element.ownerDocument.activeElement){state.unexpected++;event.preventDefault();event.stopImmediatePropagation()}
    }
    const key=(event:KeyboardEvent)=>{
     if(!event.isTrusted||event.key!=='Enter')return
     state.keys++
     if(event.target!==element||element!==element.ownerDocument.activeElement||!element.isConnected){state.unexpected++;return}
     if(paused())state.rejectedKeys++
    }
    const click=(event:MouseEvent)=>{
     if(!event.isTrusted||!(event.target instanceof Node)||!element.contains(event.target))return
     const rejected=paused()
     // Base UI's focusable disabled native button emits a browser click but
     // prevents its default and never calls the supplied onClick handler.
     if(!event.defaultPrevented)state.accepted++;else if(rejected)state.blocked++;else state.unexpected++
    }
    if(recordScope){element.ownerDocument.addEventListener('keydown',originalGuard,true);element.ownerDocument.addEventListener('click',originalGuard,true)}
    element.ownerDocument.addEventListener('keydown',key,true);element.ownerDocument.addEventListener('click',click)
    return {state,dispose:()=>{element.ownerDocument.removeEventListener('keydown',originalGuard,true);element.ownerDocument.removeEventListener('click',originalGuard,true);element.ownerDocument.removeEventListener('keydown',key,true);element.ownerDocument.removeEventListener('click',click)}}
   },declared??null)
   let observed:{keys:number;rejectedKeys:number;accepted:number;blocked:number;unexpected:number}
   try{await control.page().keyboard.press('Enter');observed=await observation.evaluate(value=>value.state)}
   finally{await observation.evaluate(value=>value.dispose());await observation.dispose()}
   expect(mutations,'Opening a local step must not send an HTTP mutation').toEqual([])
   if(observed.accepted===1&&observed.keys===1&&observed.unexpected===0){
    if(outcome==='focused')await expect(destination).toBeFocused();else await expect(destination).toBeVisible()
    expect(mutations,'A local destination must not send an HTTP mutation').toEqual([])
    return
   }
   if(attempt!==0||observed.keys!==1||observed.rejectedKeys!==1||observed.accepted!==0||observed.blocked>1||observed.unexpected!==0)throw new Error('Native inspection was not positively rejected by a current read; no navigation was repeated')
   await expect(control).toBeEnabled();await same()
  }
 }finally{control.page().off('request',request);await declared?.dispose();await original.dispose()}
}

export async function openLocal(control:Locator,destination:Locator,outcome:'focused'|'visible'='focused'){
 await openLocalStep(control,destination,outcome)
}

// Exact safe original request only: never a private-workspace/body fallback.
export async function readOriginalAcknowledgement(control:Locator,destination:Locator){
 await openLocalStep(control,destination,'visible',true)
}

export async function inspectCurrent(control:Locator,destination:Locator){
 if(!await control.getAttribute('data-record-focus'))throw new Error('Local inspection requires an explicit current record identity')
 await openLocal(control,destination)
}
