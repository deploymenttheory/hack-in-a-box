import { StatusBadge } from './components/ui/feedback'
import { localDate } from './event-time'
import { stateName, useDelivery, type Delivery } from './delivery-context'

const channelName = { smtp: 'SMTP email', teams: 'Teams shared channel', in_app: 'Private in-app inbox' }
export function DeliverySummary({ item }: { item: Delivery }) {
  const { event } = useDelivery()
  return <><p><StatusBadge tone={item.state === 'ambiguous' ? 'warning' : 'neutral'}>{stateName(item.state)}</StatusBadge> · {channelName[item.channel]}</p><p>{item.campaign_id ? `Campaign version ${item.campaign_version}` : 'Connection test'} · {item.purpose.replaceAll('_', ' ')} · Selected {localDate(item.selected_at, event.configuration.timezone)}</p><p>{item.attempts} prepared provider attempts · {item.reason.replaceAll('_', ' ')}</p>{item.cancelled && <p className="notice">This operation is stopped. Any previous provider acceptance or uncertain effect is retained and was not recalled.</p>}</>
}
