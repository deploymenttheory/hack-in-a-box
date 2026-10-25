import { readFileSync } from 'node:fs'
import { request } from 'node:http'
import type { Browser, BrowserContext } from '@playwright/test'
const manifest = process.env.HACKBOX_REPORT_MANIFEST
if (!manifest) throw new Error('Run through native Go TestTeamReportsUIBrowsers after building both production portals.')
export const native = JSON.parse(readFileSync(manifest, 'utf8')) as { event_id: string; team_id: string; admin: string; public: string; socket: string; viewport: 'desktop' | 'mobile'; ballot_event_id: string; ballot_team_id: string; second_team_id: string; ballot_admin: string; ballot_public: string }
export function control<T = unknown>(action: string, extra: Record<string, string> = {}): Promise<T> {
 return new Promise((resolve, reject) => {
  const body = JSON.stringify({ action, ...extra }), req = request({ socketPath: native.socket, path: '/', method: 'POST', headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body) } }, response => {
   let text = ''; response.setEncoding('utf8').on('data', chunk => { text += chunk }); response.on('end', () => { if (!response.statusCode || response.statusCode >= 300) reject(new Error(`Native private report control ${response.statusCode}: ${text}`)); else resolve((text ? JSON.parse(text) : undefined) as T) })
  }); req.on('error', reject); req.end(body)
 })
}
export async function actor(browser: Browser, surface: 'admin' | 'public', role: 'competitor' | 'owner' | 'judge' | 'judge2', edition: 'reports' | 'ballots' = 'reports'): Promise<BrowserContext> {
 const cookie = await control<{ Name: string; Value: string }>('session', { surface, actor: role, edition }), origin = edition === 'ballots' ? native[surface === 'admin' ? 'ballot_admin' : 'ballot_public'] : native[surface], context = await browser.newContext({ baseURL: origin, viewport: native.viewport === 'mobile' ? { width: 390, height: 844 } : { width: 1440, height: 1000 } })
 await context.addCookies([{ name: cookie.Name, value: cookie.Value, url: origin, httpOnly: true, sameSite: 'Lax' }]); return context
}
