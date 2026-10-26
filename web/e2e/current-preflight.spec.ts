import { createServer } from 'node:http'
import { test, expect } from '@playwright/test'
import { heldCurrentReads } from '../e2e-support/held-preflight'

// Real loopback HTTP/browser transport proof of the harness, separate from
// native domain proof. No production fetch replacement or invented API success.
async function server() {
 const state = { source: 'original', reads: 0 }
 const http = createServer((request, response) => {
  if (request.url === '/current') { state.reads++; response.setHeader('content-type', 'text/plain'); response.end(state.source); return }
  response.setHeader('content-type', 'text/html'); response.end('<button id="confirm">Confirm current source</button><output id="result"></output><script>const button=document.getElementById("confirm"),result=document.getElementById("result");button.onclick=async()=>{button.setAttribute("aria-busy","true");try{result.textContent=await(await fetch("/current")).text()}finally{button.setAttribute("aria-busy","false")}};window.poll=()=>fetch("/current").then(r=>r.text())</script>')
 })
 await new Promise<void>(resolve => http.listen(0, '127.0.0.1', resolve))
 const address = http.address(); if (!address || typeof address === 'string') throw new Error('Real loopback HTTP address unavailable')
 return { state, origin: `http://127.0.0.1:${address.port}`, close: () => new Promise<void>((resolve, reject) => http.close(error => error ? reject(error) : resolve())) }
}
test('request timing start is zero inside interception before its real response', async ({ page }) => {
 const http = await server(); let start = -1
 try {
  await page.goto(http.origin)
  await page.route('**/current', async route => { start = route.request().timing().startTime; const response = await route.fetch(); await route.fulfill({ response }) })
  await page.getByRole('button', { name: 'Confirm current source' }).click()
  await expect(page.locator('output')).toHaveText('original')
  expect(start).toBe(0); expect(http.state.reads).toBe(1)
 } finally { await http.close() }
})
test('trusted-click gate holds actual preflight and coincident polls until their real HTTP source changes', async ({ page }) => {
 const http = await server()
 try {
  await page.goto(http.origin)
  const button = page.getByRole('button', { name: 'Confirm current source' }), gate = await heldCurrentReads(page, '**/current', button)
  try {
   expect(await page.evaluate(() => (window as unknown as { poll: () => Promise<string> }).poll())).toBe('original')
   let reached = false; void gate.pending.then(() => { reached = true })
   await button.click(); await expect.poll(() => reached).toBe(true)
   await expect(button).toHaveAttribute('aria-busy', 'true'); expect(http.state.reads).toBe(1)
   // Ordinary polls still run, but their actual HTTP transport joins the
   // bounded gate. There is no ambiguous single-request claimant or timer stop.
   const polls = page.evaluate(() => Promise.all([(window as unknown as { poll: () => Promise<string> }).poll(), (window as unknown as { poll: () => Promise<string> }).poll()]))
   await expect.poll(() => gate.waiting).toBe(3); expect(http.state.reads).toBe(1)
   http.state.source = 'replacement'; gate.release()
   expect(await polls).toEqual(['replacement', 'replacement'])
   await expect(page.locator('output')).toHaveText('replacement'); expect(http.state.reads).toBe(4)
  } finally { await gate.dispose() }
 } finally { await http.close() }
})
