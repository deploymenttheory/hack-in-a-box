import { readFileSync } from 'node:fs'
import { request } from 'node:http'
import type { Browser, BrowserContext } from '@playwright/test'

type Cookie = { Name: string; Value: string }
type Manifest = { event_id: string; admin: string; public: string; display: string; socket: string; mc_cookie: Cookie; mentor_cookie: Cookie; competitor_cookie: Cookie; run_id: string }
const path = process.env.HACKBOX_REHEARSAL_MANIFEST
if (!path) throw new Error('Run through the Go TestLiveRehearsalBrowsers owner; native servers and PostgreSQL are required.')
export const native = JSON.parse(readFileSync(path, 'utf8')) as Manifest

export async function actor(browser: Browser, surface: 'admin' | 'public', cookie: Cookie): Promise<BrowserContext> {
  const context = await browser.newContext({ baseURL: native[surface] })
  await context.addCookies([{ name: cookie.Name, value: cookie.Value, url: native[surface], httpOnly: true, sameSite: 'Lax' }])
  return context
}

export function control<T = unknown>(action: string, extra: Record<string, unknown> = {}): Promise<T> {
  return new Promise((resolve, reject) => {
    const data = JSON.stringify({ action, ...extra })
    const req = request({ socketPath: native.socket, path: '/', method: 'POST', headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(data) } }, response => {
      let body = ''
      response.setEncoding('utf8').on('data', value => { body += value })
      response.on('end', () => {
        if (!response.statusCode || response.statusCode >= 300) reject(new Error(`Private harness control ${action}: ${response.statusCode} ${body}`))
        else resolve((body ? JSON.parse(body) : undefined) as T)
      })
    })
    req.on('error', reject)
    req.end(data)
  })
}
