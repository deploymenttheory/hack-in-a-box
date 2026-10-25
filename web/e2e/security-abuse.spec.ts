import { expect, test } from '@playwright/test'
import { firstID, mockWorkspace } from './fixtures'
import type { components } from '../src/api/schema.gen'

test('released malicious Markdown cannot execute, auto-load assets or retain content after current denial', async ({ page }) => {
 await mockWorkspace(page)
 const id = '7924b2c4-b4bb-4f38-901c-af53de9787b1', path = `/api/v1/events/${firstID}/content/${id}`
 const requests: string[] = [], dialogs: string[] = []
 page.on('request', request => { if (request.url().includes('security121-attacker.invalid')) requests.push(request.url()) })
 page.on('dialog', dialog => { dialogs.push(dialog.type()); void dialog.dismiss() })
 await page.addInitScript(() => { Object.assign(window, { security121Executed: false }) })
 const markdown = [
  '# Synthetic released safety example', 'PRIVATE-SECURITY121-RELEASED-CONTENT',
  '[Safe reference](https://example.invalid/approved)', '[Active scheme](javascript:alert%281%29)',
  '[Encoded scheme](javascript&#58;alert%281%29)', '[Data payload](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)',
  '[Credential URL](https://synthetic:private@security121-attacker.invalid/path)',
  '![Remote tracker](https://security121-attacker.invalid/tracker.png)',
  '<script>window.security121Executed=true</script>',
  '<img src="https://security121-attacker.invalid/raw.png" onerror="window.security121Executed=true">',
  '<svg onload="window.security121Executed=true"><a href="javascript:alert(1)">active</a></svg>',
  '<iframe srcdoc="<script>parent.security121Executed=true</script>"></iframe>',
  '<style>@import url(https://security121-attacker.invalid/style.css);</style>',
 ].join('\n\n')
 const content: components['schemas']['ContentPage'] = { id, kind: 'page', revision: 1, version: 1, state: 'released', released_version: 1, released_at: '2027-06-01T12:00:00Z', acknowledged_at: null, can_manage: false, attachment_details: [], message: '', document: { title: 'Synthetic malicious Markdown', markdown, require_acknowledgement: false, audience: { confirmed: true, roles: [], teams: [] }, attachments: [], scheduled_at: null } }
 let allowed = true
 await page.route('**' + path, route => route.fulfill(allowed ? { json: content } : { status: 403, json: { error: { code: 'forbidden', message: 'Current synthetic audience revoked', request_id: 'synthetic' } } }))
 await page.goto(`/events/${firstID}/resources/${id}`)
 const article = page.getByRole('article', { name: 'Released resource text', exact: true })
 await expect(article).toContainText('PRIVATE-SECURITY121-RELEASED-CONTENT')
 const safe = article.getByRole('link', { name: 'Safe reference', exact: true })
 await expect(safe).toHaveAttribute('href', 'https://example.invalid/approved')
 await expect(safe).toHaveAttribute('rel', 'noopener noreferrer')
 await expect(article.getByRole('link')).toHaveCount(1)
 await expect(article.locator('script,img,svg,iframe,style,object,embed')).toHaveCount(0)
 await expect(article).toContainText('Image: Remote tracker')
 expect(await page.evaluate(() => (window as unknown as { security121Executed: boolean }).security121Executed)).toBe(false)
 expect(requests).toEqual([]); expect(dialogs).toEqual([])
 allowed = false
 const denied = page.waitForResponse(response => new URL(response.url()).pathname === path && response.status() === 403)
 await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
 await denied
 await expect(article).toHaveCount(0)
 await expect(page.getByText('PRIVATE-SECURITY121-RELEASED-CONTENT', { exact: false })).toHaveCount(0)
 expect(requests).toEqual([]); expect(dialogs).toEqual([])
})
