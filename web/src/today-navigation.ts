// Deployment-validated origin only. URL input never grants a capability: the
// destination portal selects from its own current, permitted section registry.
export function administrationSectionURL(origin: string | undefined, eventID: string, section: string) {
  if (!origin) return undefined
  try {
    const url = new URL(origin)
    if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password || url.pathname !== '/' || url.search || url.hash) return undefined
    url.pathname = `/events/${encodeURIComponent(eventID)}`
    url.searchParams.set('section', section)
    return url.toString()
  } catch { return undefined }
}
export function initialEventSection(search: string) {
  const section = new URLSearchParams(search).get('section')
  return section && /^[a-z ]{1,40}$/.test(section) ? section : 'overview'
}
