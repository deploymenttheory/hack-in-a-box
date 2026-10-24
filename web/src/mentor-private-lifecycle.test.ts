import { QueryClient } from '@tanstack/react-query'
import { expect, test } from 'vitest'
import { scrubMentorPrivateQueries } from './mentor-private-lifecycle'

test('parent observation denial removes only its owned team detail and leaves unrelated sources intact', () => {
  const cache = new QueryClient()
  const privateNote = ['private', 'actor', 'event', 'mentor-exact-note', 'team', 'note']
  const parent = ['private', 'actor', 'event', 'mentor-observations', 'team']
  const otherTeam = ['private', 'actor', 'event', 'mentor-exact-note', 'other-team', 'note']
  const otherActor = ['private', 'other', 'event', 'mentor-exact-note', 'team', 'note']
  const otherDomain = ['private', 'actor', 'event', 'help-request', 'request']
  for (const key of [privateNote, parent, otherTeam, otherActor, otherDomain]) cache.setQueryData(key, { canary: 'private snapshot' })
  scrubMentorPrivateQueries(cache, 'actor', 'event', 'team', 'observations')
  expect(cache.getQueryData(privateNote)).toBeUndefined()
  for (const key of [parent, otherTeam, otherActor, otherDomain]) expect(cache.getQueryData(key)).toEqual({ canary: 'private snapshot' })
})

test('a held private child read cancelled at parent loss cannot refill the removed detail cache', async () => {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const key = ['private', 'actor', 'event', 'scorecard-current', 'team', 'checkpoint']
  let finish!: (value: { canary: string }) => void
  const pending = cache.fetchQuery({ queryKey: key, queryFn: () => new Promise<{ canary: string }>(resolve => { finish = resolve }) }).catch(() => undefined)
  scrubMentorPrivateQueries(cache, 'actor', 'event', 'team', 'scorecards')
  finish({ canary: 'LATE PRIVATE SCORE' }); await pending
  expect(cache.getQueryData(key)).toBeUndefined()
})
