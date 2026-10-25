import { readFileSync } from 'node:fs'
import type { Browser, BrowserContext } from '@playwright/test'
import { control } from '../live-rehearsal/native'

type Manifest = { admission?: { event_id: string; form_id: string; instant: string; public: string; admin: string } }
const path = process.env.HACKBOX_REHEARSAL_MANIFEST
if (!path) throw new Error('Use the native accessibility Go owner; no mocked admission server is provided.')
const fixture = (JSON.parse(readFileSync(path, 'utf8')) as Manifest).admission
if (!fixture) throw new Error('The isolated pre-admission fixture is required.')
export const admission = fixture
export async function admissionActor(browser: Browser, actor: 'applicant' | 'captain' | 'teammate' | 'unknown-joiner', surface: 'public' | 'admin' = 'public'): Promise<BrowserContext> {
  const cookie = await control<{ Name: string; Value: string }>('admission-session', { actor, surface })
  const context = await browser.newContext({ baseURL: admission[surface] })
  await context.addCookies([{ name: cookie.Name, value: cookie.Value, url: admission[surface], httpOnly: true, sameSite: 'Lax' }])
  return context
}
