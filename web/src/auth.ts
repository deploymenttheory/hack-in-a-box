export function loginURL(path = window.location.pathname + window.location.search) {
  const destination = path.startsWith('/') && !path.startsWith('//') && !path.includes('\\') ? path : '/'
  return `/auth/login?${new URLSearchParams({ return_to: destination })}`
}

const draftPrefix = 'hackbox:draft:'
export function clearPrivateDrafts(exceptPerson?: string) {
  for (const storage of [window.sessionStorage, window.localStorage]) {
    for (let index = storage.length - 1; index >= 0; index--) {
      const key = storage.key(index)
      if (key?.startsWith(draftPrefix) && (!exceptPerson || !key.startsWith(`${draftPrefix}${exceptPerson}:`))) storage.removeItem(key)
    }
  }
}

// Only explicitly safe event-configuration fields belong here. Personal
// responses, credentials and assessments must never enter browser storage.
export function safeDraftKey(person: string, resource: string) { return `${draftPrefix}${person}:${resource}` }
