import { readFileSync } from 'node:fs'
import { request } from 'node:http'
import type { Browser, BrowserContext, Page } from '@playwright/test'
type Cookie = { Name: string; Value: string }
type Manifest = { origin: string; socket: string; scope: string; event_id: string; second_event_id: string; winner_project: string; nonwinner_project: string; second_project: string; draft_project: string; restricted_project: string; media_id: string }
const path = process.env.HACKBOX_SHOWCASE_MANIFEST
if (!path) throw new Error('Actual native showcase PostgreSQL/HTTP fixture required.')
export const native = JSON.parse(readFileSync(path, 'utf8')) as Manifest
export async function actor(browser: Browser, role: 'competitor' | 'outsider' | 'owner' = 'competitor'): Promise<BrowserContext> {
 const cookie = await control<Cookie>('session', { actor: role })
 const mobile = native.scope.endsWith('mobile')
 const context = await browser.newContext({ baseURL: native.origin, viewport: mobile ? { width: 390, height: 844 } : { width: 1440, height: 1000 }, isMobile: mobile, hasTouch: mobile })
 await context.addCookies([{ name: cookie.Name, value: cookie.Value, url: native.origin, httpOnly: true, sameSite: 'Lax' }])
 return context
}
export function control<T = unknown>(action: string, extra: { actor?: string } = {}): Promise<T> {
 return new Promise((resolve, reject) => {
  const data = JSON.stringify({ action, ...extra })
  const req = request({ socketPath: native.socket, path: '/', method: 'POST', headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(data) } }, response => {
   let body = ''; response.setEncoding('utf8').on('data', value => { body += value }); response.on('end', () => {
    if (!response.statusCode || response.statusCode >= 300) reject(new Error(`Owner-private native ${action}: ${response.statusCode} ${body}`))
    else resolve((body ? JSON.parse(body) : undefined) as T)
   })
  }); req.on('error', reject); req.end(data)
 })
}

// Theme media changes may start finite production background transitions. Audit
// their actual completed palette rather than a transient interpolation frame.
export async function settleTheme(page: Page) {
 await page.locator('main').evaluate(async element => {
  getComputedStyle(element).getPropertyValue('--background')
  const transitions = element.getAnimations({ subtree: true }).filter(animation => {
   const timing = animation.effect?.getComputedTiming()
   return timing && Number.isFinite(timing.endTime)
  })
  await Promise.all(transitions.map(animation => animation.finished.catch(() => {})))
 })
}
