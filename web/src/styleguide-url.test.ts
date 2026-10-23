import { expect, test } from 'vitest'
import { resolveStyleguideURL } from './styleguide-url'
test('both portals share a public deployment configured docs URL', () => {
  expect(resolveStyleguideURL()).toBe('http://127.0.0.1:8082')
  expect(resolveStyleguideURL('https://docs.example.test/design/')).toBe('https://docs.example.test/design/')
  for (const value of ['javascript:alert(1)', 'https://user:secret@example.test/', '/private-route']) expect(() => resolveStyleguideURL(value)).toThrow()
})
