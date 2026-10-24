import type { QueryClient } from '@tanstack/react-query'

const observationSources = new Set(['mentor-exact-note', 'mentor-history', 'scorecard-observations', 'scorecard-exact-evidence'])
const scorecardSources = new Set(['scorecard-current', 'scorecard-history', 'scorecard-observations', 'scorecard-submission', 'scorecard-exact-evidence'])
const huddleSources = new Set(['private-huddle', 'huddle-feedback-actions', 'huddle-history'])
// Scrub only this actor/event/team's owned private detail caches. The owning
// collection query is deliberately excluded: it must finish its current denial
// or restricted projection. Unknown-command history is a separate memory owner.
export function scrubMentorPrivateQueries(cache: QueryClient, actor: string, eventID: string, teamID: string, domain: 'observations' | 'scorecards' | 'huddles') {
  const sources = domain === 'observations' ? observationSources : domain === 'scorecards' ? scorecardSources : huddleSources
  const predicate = (query: { queryKey: readonly unknown[] }) => query.queryKey[0] === 'private' && query.queryKey[1] === actor && query.queryKey[2] === eventID && sources.has(String(query.queryKey[3])) && query.queryKey.slice(4).includes(teamID)
  void cache.cancelQueries({ predicate })
  cache.removeQueries({ predicate })
}
