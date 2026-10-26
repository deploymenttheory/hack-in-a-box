import { expect, type Page, type Locator, type Route } from '@playwright/test'

// The trusted confirmation marker precedes its production click handler. Hold
// subsequent current GET transport, including any coincident ordinary poll,
// until the native source is replaced. Poll timers remain active; this fixture
// deliberately delays their transport too and does not claim to identify an
// indistinguishable preflight/poll request. Every response is fetched natively.
export async function heldCurrentReads(page: Page, path: string, confirm: Locator) {
 let release!: () => void, started!: () => void, cancelled = false, waiting = 0
 const resume = new Promise<void>(resolve => { release = resolve }), pending = new Promise<void>(resolve => { started = resolve }), key = `__nativePreflight_${Math.random()}`
 await confirm.evaluate((element, key) => {
  const state = { clicked: false }
  const clicked = (event: Event) => { if (event.isTrusted) state.clicked = true }
  element.addEventListener('click', clicked, { capture: true, once: true })
  Object.assign(window, { [key]: { state, dispose: () => { element.removeEventListener('click', clicked, true) } } })
 }, key)
 const handler = async (route: Route) => {
  if (route.request().method() !== 'GET') return route.fallback()
  const clicked = await page.evaluate(key => (window as unknown as Record<string, { state: { clicked: boolean } }>)[key]?.state.clicked, key)
  if (!clicked) return route.fallback()
  waiting++; started(); await resume
  if (cancelled) { await route.abort('aborted'); return }
  const response = await route.fetch(); expect(response.status()).toBe(200); await route.fulfill({ response })
 }
 await page.route(path, handler)
 return { pending, release, get waiting() { return waiting }, dispose: async () => { cancelled = true; release(); await page.unroute(path, handler); await page.evaluate(key => { const values = window as unknown as Record<string, { dispose: () => void }>; values[key]?.dispose(); delete values[key] }, key) } }
}
