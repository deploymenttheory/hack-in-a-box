import { expect, test } from 'vitest'
import { administrationSectionURL, initialEventSection } from './today-navigation'
test('administration links contain only the trusted origin and explicitly encoded event and section', () => {
 expect(administrationSectionURL('https://admin.example.test', 'event-id', 'mc console')).toBe('https://admin.example.test/events/event-id?section=mc+console')
 for (const origin of [undefined, 'javascript:alert(1)', 'https://user:password@example.test', 'https://example.test/path', 'https://example.test?redirect=evil', 'https://example.test#private', 'https://example.test?', 'https://example.test#']) expect(administrationSectionURL(origin, 'event-id', 'content')).toBeUndefined()
})
test('section input is bounded and remains only a request for the current portal registry', () => {
 expect(initialEventSection('?section=mc+console')).toBe('mc console')
 for (const search of ['?section=https://evil.test', '?section=CONTENT', '?section=%3Cscript%3E', '?section=' + 'a'.repeat(41)]) expect(initialEventSection(search)).toBe('overview')
})
