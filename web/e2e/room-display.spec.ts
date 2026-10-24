import { contrastOf } from './design-system-contrast'
import { contrast } from '../src/branding'
import { test, expect, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import jsQR from 'jsqr'
import type { components } from '../src/api/schema.gen'
const eventID = '00000000-0000-4000-8000-000000000001', assetID='00000000-0000-4000-8000-000000000002'
const token = 'SYNTHETIC-DISPLAY-CAPABILITY-NEVER-IN-URL'
const raster = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=','base64')
function initial(): components['schemas']['DisplaySnapshot'] { return { event_id:eventID,event_name:'Synthetic summer hack',timezone:'Europe/London',sequence:7,server_now:'2030-06-14T09:00:00Z',current:{id:eventID,title:'Build something useful',text:'Work with your team.\nAsk a mentor when you need help.',url:'https://example.com/challenge?edition=summer',content_title:'Released challenge'},next:{id:assetID,title:'Share your progress',text:'',url:'',content_title:''},mode:'running',remaining_seconds:120,timer_ends_at:'2030-06-14T09:02:00Z',primary_colour:'#173a35',background_colour:'#f8f9f5',foreground_colour:'#173a35',override_active:false,assets:[] } }
async function room(page: Page, palette: Partial<components['schemas']['DisplaySnapshot']> = {}) {
 const state = { snapshot:{...initial(),...palette},offline:false,revoked:false,assetMissing:false, streams:0, frameOverride:'', unexpected:[] as string[] }
 await page.clock.install({time:new Date('2027-06-14T09:00:00Z')})
 await page.route('**/api/**',route => {state.unexpected.push(route.request().url());return route.fulfill({status:500})})
 await page.route('**/display/stream',route => {
  state.streams++
  expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
  expect(route.request().headers().cookie).toBeUndefined()
  expect(route.request().url()).not.toContain(token)
  if(state.offline)return route.abort('failed')
  return route.fulfill({contentType:'text/event-stream',headers:{'Cache-Control':'no-store'},body:state.frameOverride || (state.revoked?'event: revoked\ndata: {}\n\n':`id: ${state.snapshot.sequence}\nevent: snapshot\ndata: ${JSON.stringify({...state.snapshot, private_assessment:'PRIVATE FIELD MUST NOT RENDER'})}\n\n`)} )
 })
 await page.route('**/display/assets/*',route => {expect(route.request().headers().authorization).toBe(`Bearer ${token}`);expect(route.request().headers().cookie).toBeUndefined();return state.assetMissing?route.fulfill({status:404}):route.fulfill({contentType:'image/png',body:raster})})
 await page.goto(`/display/${eventID}`)
 await page.getByLabel('Display key',{exact:true}).fill(token)
 await page.getByRole('button',{name:'Connect display',exact:true}).click()
 await expect(page.getByRole('heading',{name:state.snapshot.current!.title,exact:true})).toBeVisible()
 return state
}
async function noStoredCapability(page: Page) {
 const values=await page.evaluate(()=>[...Object.values(localStorage),...Object.values(sessionStorage),location.href].join('\n'))
 expect(values).not.toContain(token)
}
test('renders a real QR matching the safe visible destination without account authority',async({page})=>{
 const state=await room(page)
 const link=page.locator('.room-qr a'),href=await link.getAttribute('href')
 expect(href).toBe('https://example.com/challenge?edition=summer')
 const pixels=await page.locator('.room-qr svg').evaluate(async svg=>{
  const image=new Image(),canvas=document.createElement('canvas');canvas.width=512;canvas.height=512
  image.src=`data:image/svg+xml;charset=utf-8,${encodeURIComponent(new XMLSerializer().serializeToString(svg))}`
  await new Promise<void>((resolve,reject)=>{image.onload=()=>resolve();image.onerror=()=>reject(new Error('qr_image_failed'))})
  const context=canvas.getContext('2d')!;context.drawImage(image,0,0,512,512)
  return Array.from(context.getImageData(0,0,512,512).data)
 })
 expect(jsQR(new Uint8ClampedArray(pixels),512,512)?.data).toBe(href)
 expect(state.unexpected).toEqual([])
 await expect(page.getByText('PRIVATE FIELD MUST NOT RENDER')).toHaveCount(0)
 await noStoredCapability(page)
})
test('replaces withdrawn content and branding at the same cue sequence',async({page},testInfo)=>{
 const state=await room(page)
 state.snapshot={...state.snapshot,current:null,next:null,background_colour:'#18222b',foreground_colour:'#ffffff',primary_colour:'#ffffff',assets:[{id:assetID,kind:'logo',media_type:'image/png',digest:createHash('sha256').update(raster).digest('hex')}]}
 await page.clock.runFor(1_100)
 await expect(page.getByRole('heading',{name:'Waiting for the MC',exact:true})).toBeVisible()
 await expect(page.getByText('Released challenge',{exact:true})).toHaveCount(0)
 await expect(page.locator('.room-qr')).toHaveCount(0)
 await expect(page.getByAltText('Event logo',{exact:true})).toBeVisible()
 await expect(page.locator('.room-display')).toHaveCSS('background-color','rgb(24, 34, 43)')
 await expect(page.locator('.room-display footer')).toHaveCSS('color','rgb(255, 255, 255)')
 await page.screenshot({path:testInfo.outputPath('room-dark-theme.png'),fullPage:true})
 state.snapshot.assets=[]
 await page.clock.runFor(1_100)
 await expect(page.getByAltText('Event logo',{exact:true})).toHaveCount(0)
})
test('connection loss retains the last safe cue and server-clock countdown until reconnect or revocation',async({page})=>{
 const state=await room(page)
 await expect(page.locator('.room-countdown')).toHaveText('02:00')
 state.offline=true
 await page.clock.runFor(20_000)
 await expect(page.getByRole('status')).toContainText('Connection lost')
 await expect(page.getByRole('status')).toContainText('Last update')
 await expect(page.getByRole('heading',{name:'Build something useful',exact:true})).toBeVisible()
 await expect(page.locator('.room-countdown')).toHaveText('01:40')
 state.offline=false;state.revoked=true
 await page.clock.runFor(16_000)
 await expect(page.getByRole('heading',{name:'Connect a room display',exact:true})).toBeVisible()
 await expect(page.getByText('Build something useful',{exact:true})).toHaveCount(0)
 await expect(page.getByRole('status')).toContainText('pairing has ended')
 await noStoredCapability(page)
})
test('unsafe destinations and missing images retain readable text defaults',async({page})=>{
 const state=await room(page)
 state.assetMissing=true
 state.snapshot={...state.snapshot,primary_colour:'url(javascript:alert(1))',current:{...state.snapshot.current!,url:'javascript:alert(1)'},assets:[{id:assetID,kind:'hero',media_type:'image/png',digest:createHash('sha256').update(raster).digest('hex')}]}
 await page.clock.runFor(1_100)
 await expect(page.locator('.room-qr')).toHaveCount(0)
 await expect(page.locator('.room-display img')).toHaveCount(0)
 await expect(page.locator('.room-display')).toHaveCSS('background-color','rgb(255, 255, 255)')
 await expect(page.getByRole('heading',{name:'Build something useful',exact:true})).toBeVisible()
})
test('room composition stays legible at 1080p and 720p',async({page},testInfo)=>{
 await room(page)
 await page.screenshot({path:testInfo.outputPath('room-project-default.png'),fullPage:true})
 for(const [width,height] of [[1920,1080],[1280,720]]){
  await page.setViewportSize({width,height})
  await expect(page.locator('.room-copy h1')).toBeVisible()
  const metrics=await page.evaluate(()=>({scroll:document.documentElement.scrollWidth,width:innerWidth,title:parseFloat(getComputedStyle(document.querySelector('.room-copy h1')!).fontSize),body:parseFloat(getComputedStyle(document.querySelector('.room-instructions')!).fontSize)}))
  expect(metrics.scroll).toBeLessThanOrEqual(metrics.width);expect(metrics.title).toBeGreaterThanOrEqual(48);expect(metrics.body).toBeGreaterThanOrEqual(30)
  await page.screenshot({path:testInfo.outputPath(`room-${width}x${height}.png`),fullPage:true})
 }
})

test('renders a maximum legal Go-escaped current and next snapshot without reconnect rejection',async({page})=>{
 const state=await room(page)
 state.frameOverride=readFileSync('e2e/fixtures/max-display-frame.sse','utf8')
 await page.clock.runFor(1_100)
 await expect(page.getByRole('heading',{name:'<'.repeat(200),exact:true})).toBeVisible()
 await expect(page.locator('.room-instructions')).toHaveText('<'.repeat(4000))
 await expect(page.locator('.room-display footer strong')).toHaveText('<'.repeat(200))
 await expect(page.locator('.room-countdown')).toHaveText('02:00')
})

for (const palette of [
 { name: 'native light room under system dark', background_colour: '#f8f9f5', foreground_colour: '#173a35', primary_colour: '#173a35' },
 { name: 'threshold grey room under system dark', background_colour: '#ffffff', foreground_colour: '#767676', primary_colour: '#767676' },
]) test(`${palette.name} derives readable room action roles and native controls`, async ({ page }, info) => {
 await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' })
 await room(page, palette)
 const display = page.locator('.room-display'), control = page.getByRole('button', { name: 'Fullscreen', exact: true })
 const roles = await display.evaluate(el => { const s = getComputedStyle(el); return { primary: s.getPropertyValue('--primary').trim(), foreground: s.getPropertyValue('--primary-foreground').trim(), scheme: s.colorScheme } })
 expect(contrast(roles.primary, roles.foreground)).toBeGreaterThanOrEqual(4.5)
 expect(roles.scheme).toBe('light')
 expect(await contrastOf(control)).toBeGreaterThanOrEqual(4.5)
 expect(await contrastOf(control, 'border-top-color')).toBeGreaterThanOrEqual(3)
 await control.hover(); expect(await contrastOf(control)).toBeGreaterThanOrEqual(4.5)
 await page.keyboard.press('Tab'); await control.focus()
 expect(await control.evaluate(el => el.matches(':focus-visible'))).toBe(true)
 await expect(control).toHaveCSS('outline-style', 'solid'); await expect(control).toHaveCSS('outline-width', '2px')
 expect(await contrastOf(control, 'outline-color')).toBeGreaterThanOrEqual(3)
 await page.screenshot({ path: info.outputPath('room-control-theme.png'), fullPage: true })
})

test('a held authorised asset cannot create a URL after equal-sequence asset withdrawal',async({page})=>{
 const state=await room(page)
 let started=false,release!:()=>void
 const held=new Promise<void>(resolve=>{release=resolve})
 await page.evaluate(()=>{const original=URL.createObjectURL;Object.assign(window,{displayCreatedURLs:0});URL.createObjectURL=function(blob){(window as unknown as {displayCreatedURLs:number}).displayCreatedURLs++;return original(blob)}})
 await page.route('**/display/assets/*',async route=>{started=true;await held;return route.fulfill({contentType:'image/png',body:raster})})
 try{
  state.snapshot={...state.snapshot,assets:[{id:assetID,kind:'logo',media_type:'image/png',digest:createHash('sha256').update(raster).digest('hex')}]}
  await page.clock.runFor(1_100);await expect.poll(()=>started).toBe(true)
  state.snapshot={...state.snapshot,assets:[]}
  await page.clock.runFor(1_100)
  release();await page.clock.runFor(100)
  await expect(page.getByAltText('Event logo')).toHaveCount(0)
  expect(await page.evaluate(()=>(window as unknown as {displayCreatedURLs:number}).displayCreatedURLs)).toBe(0)
 }finally{release()}
})
