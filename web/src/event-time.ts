export function localDate(value: string | null | undefined, timezone: string) {
  if (!value || value.startsWith('0001-')) return 'Not set'
  return new Intl.DateTimeFormat('en-GB', { timeZone: timezone, dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

export function wallTime(value?: string, timezone = 'Europe/London') {
  if (!value || value.startsWith('0001-')) return ''
  const parts = new Intl.DateTimeFormat('en-GB', { timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }).formatToParts(new Date(value))
  const get = (key: string) => parts.find(p => p.type === key)?.value
  return `${get('year')}-${get('month')}-${get('day')}T${get('hour')}:${get('minute')}:${get('second')}`
}
