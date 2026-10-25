import { readFileSync } from 'node:fs'
import http from 'node:http'
import type { Browser, Page } from '@playwright/test'
const manifest = process.env.HACKBOX_ADVISORY_MANIFEST
if (!manifest) throw new Error('Missing owner-private native advisory manifest')
export const native = JSON.parse(readFileSync(manifest, 'utf8')) as { origin: string; socket: string; scope: string; viewport: string; event_id: string; team_id: string; team_name: string; tie_id: string; category_name: string; run_id: string }
export async function control<T = unknown>(action: string, actor?: string): Promise<T> {
 return new Promise((resolve, reject) => { const request = http.request({ socketPath: native.socket, path: '/', method: 'POST', headers: { 'content-type': 'application/json' } }, response => { let raw = ''; response.setEncoding('utf8'); response.on('data', data => { raw += data }); response.on('end', () => { if (!response.statusCode || response.statusCode < 200 || response.statusCode >= 300) { reject(new Error(`Native control ${action}: ${response.statusCode} ${raw}`)); return } resolve(raw ? JSON.parse(raw) as T : undefined as T) }) }); request.on('error', reject); request.end(JSON.stringify({ Action: action, Actor: actor ?? '' })) })
}
export async function actor(browser: Browser, role = 'mentor') {
 const cookie = await control<{ Name: string; Value: string }>('session', role), context = await browser.newContext({ baseURL: native.origin, viewport: native.viewport === 'mobile' ? { width: 390, height: 844 } : { width: 1440, height: 1000 } })
 await context.addCookies([{ name: cookie.Name, value: cookie.Value, url: native.origin, httpOnly: true, sameSite: 'Lax' }]); return context
}
export async function settleTheme(page: Page) {
 await page.evaluate(async () => { getComputedStyle(document.documentElement).getPropertyValue('color'); await Promise.all(document.getAnimations().filter(animation => { const end = animation.effect?.getComputedTiming().endTime; return typeof end === 'number' && Number.isFinite(end) }).map(animation => animation.finished.catch(() => {}))) })
}
