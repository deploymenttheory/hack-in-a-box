import { useQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import type { components } from './api/schema.gen'
import { useIdentity } from './session'
import { Failure, Loading } from './components/states'

type Disclosure = components['schemas']['ObjectiveDisclosure']
export function ObjectiveDisclosure({ eventID, contentID, reference }: { eventID: string; contentID: string; reference: Disclosure }) {
  const identity = useIdentity()
  const current = useQuery({ queryKey: [...privateKey(identity.person.id, eventID, 'released-objective'), contentID, reference.objective.id, reference.objective.version], queryFn: async () => unwrap(await api.GET('/api/v1/events/{event_id}/content/{content_id}/objectives/{objective_id}', { params: { path: { event_id: eventID, content_id: contentID, objective_id: reference.objective.id }, query: { version: reference.objective.version } } })), refetchInterval: 15_000, refetchOnWindowFocus: 'always' })
  if (current.isPending) return <Loading />
  if (current.error) return <Failure error={current.error} retry={() => void current.refetch()} />
  const definition = current.data
  return <section className="released-objective" aria-label="Disclosed objective"><h3>Event objective · Definition version {definition.reference.version}</h3><p className="preserve-lines">{definition.description}</p><p>These are the released planning goals and evidence expectations. A target is not an achieved result.</p>{definition.measures.map(m => <section key={m.id}><h4>{m.description}</h4><dl><div><dt>Planning target</dt><dd>{m.target === null ? 'Unknown' : `${m.target}${m.unit ? ` ${m.unit}` : ''}`}</dd></div><div><dt>Measurement method</dt><dd>{m.method || 'Not specified'}</dd></div><div><dt>Evidence source for review</dt><dd>{m.evidence_source || 'Not specified'}</dd></div><div><dt>Review horizon</dt><dd>{m.review_horizon || 'Not specified'}</dd></div></dl></section>)}</section>
}
