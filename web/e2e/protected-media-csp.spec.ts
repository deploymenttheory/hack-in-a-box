import { test, expect } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { createServer, type Server } from 'node:http'

// Exercise the actual portal policy with a protected-photo-shaped binary Blob.
// The normal-image journey separately verifies the real Go host/API/identity.
test('portal image policy permits protected PNG blobs while external images and scripts stay blocked', async ({ page }, info) => {
  const source = readFileSync(new URL('../../internal/httpapi/assets.go', import.meta.url), 'utf8')
  const match = source.match(/Set\("Content-Security-Policy", ("[^\n]+")\)/)
  expect(match, 'Resolve the actual portal CSP literal; never substitute a test policy').not.toBeNull()
  const policy: string = JSON.parse(match![1])
  const image = readFileSync(new URL('../../docs/design/fleet-reference/fleet-workspace-admin.png', import.meta.url))
  let externalRequests = 0
  const external = createServer((request, response) => { externalRequests++; const script = request.url === '/blocked.js'; response.writeHead(200, { 'Content-Type': script ? 'text/javascript' : 'image/png' }); response.end(script ? 'window.externalMediaScript = true' : image) })
  const portal = createServer((_request, response) => { response.writeHead(200, { 'Content-Type': 'text/html', 'Content-Security-Policy': policy, 'X-Content-Type-Options': 'nosniff' }); response.end('<!doctype html><html lang="en"><head><title>Protected media policy</title></head><body></body></html>') })
  const listen = (server: Server) => new Promise<number>(resolve => { server.listen(0, '127.0.0.1', () => { const address = server.address(); if (!address || typeof address === 'string') throw new Error('Expected isolated TCP address'); resolve(address.port) }) })
  const close = (server: Server) => new Promise<void>(resolve => { server.closeAllConnections(); server.close(() => resolve()) })
  try {
    const externalPort = await listen(external), portalPort = await listen(portal)
    await page.goto(`http://127.0.0.1:${portalPort}`)
    const observed = await page.evaluate(async ({ png, externalOrigin }) => {
      const violations: { directive: string; blocked: string }[] = []
      document.addEventListener('securitypolicyviolation', event => violations.push({ directive: event.effectiveDirective, blocked: event.blockedURI }))
      const state = window as typeof window & { protectedBlobScript?: boolean; externalMediaScript?: boolean }
      const loadImage = (src: string) => new Promise<number>(resolve => { const image = new Image(); image.onload = () => resolve(image.naturalWidth); image.onerror = () => resolve(0); image.src = src; document.body.append(image) })
      const loadScript = (src: string) => new Promise<void>(resolve => { const script = document.createElement('script'); script.onload = () => resolve(); script.onerror = () => resolve(); script.src = src; document.body.append(script) })
      const bytes = Uint8Array.from(atob(png), value => value.charCodeAt(0))
      const photo = URL.createObjectURL(new Blob([bytes], { type: 'application/octet-stream' }))
      const script = URL.createObjectURL(new Blob(['window.protectedBlobScript = true'], { type: 'text/javascript' }))
      try {
        const protectedWidth = await loadImage(photo), externalWidth = await loadImage(`${externalOrigin}/pixel.png`)
        await loadScript(script); await loadScript(`${externalOrigin}/blocked.js`)
        await new Promise(resolve => setTimeout(resolve, 50))
        return { protectedWidth, externalWidth, blobScriptExecuted: !!state.protectedBlobScript, externalScriptExecuted: !!state.externalMediaScript, violations }
      } finally { URL.revokeObjectURL(photo); URL.revokeObjectURL(script) }
    }, { png: image.toString('base64'), externalOrigin: `http://127.0.0.1:${externalPort}` })
    await info.attach('policy-observations', { body: JSON.stringify({ policy, observed, externalRequests }), contentType: 'application/json' })
    expect(observed.protectedWidth).toBeGreaterThan(0)
    expect(observed.externalWidth).toBe(0)
    expect(observed.blobScriptExecuted).toBe(false)
    expect(observed.externalScriptExecuted).toBe(false)
    expect(externalRequests).toBe(0)
    expect(observed.violations.some(value => value.directive === 'img-src' && value.blocked.startsWith('http:'))).toBe(true)
    expect(observed.violations.some(value => value.directive.startsWith('script-src') && value.blocked === 'blob')).toBe(true)
    expect(observed.violations.some(value => value.directive.startsWith('script-src') && value.blocked.startsWith('http:'))).toBe(true)
  } finally { await close(portal); await close(external) }
})
