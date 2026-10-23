// Public build configuration only; never accepts credentials or executable URLs.
export function resolveStyleguideURL(configured?: string): string {
  if (!configured) return 'http://127.0.0.1:8082'
  const url = new URL(configured)
  if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) throw new Error('VITE_STYLEGUIDE_URL must be an HTTP(S) docs URL without credentials')
  return url.href
}
export const styleguideURL = resolveStyleguideURL(import.meta.env.VITE_STYLEGUIDE_URL)
