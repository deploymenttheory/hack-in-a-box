import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { api, privateKey, unwrap, type EventRecord, type Session } from './api/client'
import { Icon, type IconName } from './components/ui/icon'
import { Button } from './components/ui/button'
import { Input, Label } from './components/ui/fields'
import { Popover, PopoverContent, PopoverTrigger } from './components/ui/popover'
import { InboxLink } from './inbox'

export type PortalDestination = { href: string; label: string; icon: IconName }
export function permittedDestinations(session: Session, admin: boolean, event?: EventRecord): PortalDestination[] {
  return [
    { href: '/', label: 'Workspace', icon: 'layout' },
    { href: '/ideas', label: 'Ideas & projects', icon: 'book' },
    { href: '/inbox', label: 'Inbox', icon: 'list' },
    ...(!admin ? [{ href: '/register', label: 'Find events', icon: 'calendar' } as const] : []),
    { href: '/account', label: 'Account settings', icon: 'user' },
    ...(admin && session.can_create_events ? [{ href: '/events/new', label: 'Create event', icon: 'add' } as const, { href: '/templates', label: 'Event templates', icon: 'layout' } as const] : []),
    ...(event ? [
      { href: `/events/${event.id}`, label: event.name + ' workspace', icon: 'calendar' } as const,
      { href: `/events/${event.id}/surveys`, label: 'Surveys and feedback', icon: 'list' } as const,
      { href: `/events/${event.id}/media`, label: 'Event media', icon: 'layout' } as const,
      { href: `/events/${event.id}/photo-consent`, label: 'Your photo consent', icon: 'user' } as const,
      { href: `/events/${event.id}/help`, label: 'Event help', icon: 'help' } as const,
      ...(event.can_read_staff ? [{ href: `/events/${event.id}/review`, label: 'Support and assessment', icon: 'shield' } as const] : []),
    ] : []),
  ]
}
function DestinationSearch({ destinations }: { destinations: PortalDestination[] }) {
  const [open, setOpen] = useState(false), [search, setSearch] = useState('')
  const matches = destinations.filter(item => item.label.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()))
  return <Popover open={open} onOpenChange={value => { setOpen(value); if (!value) setSearch('') }}><PopoverTrigger render={<Button variant="ghost" icon="search" />}>Find a destination</PopoverTrigger><PopoverContent title="Go to a workspace" className="fleet-destination-search"><Label htmlFor="portal-destination">Search permitted destinations</Label><Input id="portal-destination" type="search" autoComplete="off" maxLength={100} value={search} onChange={event => setSearch(event.target.value)} /><p className="fleet-visually-hidden" role="status">{matches.length} matching destinations</p><ul>{matches.map(item => <li key={item.href}><a href={item.href}><Icon name={item.icon} />{item.label}</a></li>)}</ul>{!matches.length && <p>No matching destinations. Try another name.</p>}<p className="fleet-shell-note">Current access is checked when you open a destination.</p></PopoverContent></Popover>
}
export function PortalNavigation({ session, pathname }: { session: Session; pathname: string }) {
  const admin = import.meta.env.MODE === 'admin'
  const eventID = /^\/events\/([a-f0-9-]{36})(?:\/|$)/i.exec(pathname)?.[1]
  const query = useQuery({ queryKey: privateKey(session.person.id, eventID ?? '', 'event'), enabled: !!eventID,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/events/{event_id}', { params: { path: { event_id: eventID! } }, signal })), refetchOnWindowFocus: 'always', retry: false })
  // A failed current read withdraws the contextual destinations, even if a prior
  // response remains cached. A hostname or old role never grants navigation.
  const event = eventID && !query.error ? query.data : undefined
  const destinations = permittedDestinations(session, admin, event)
  const top = destinations.filter(item => ['/', '/ideas', '/inbox', '/register'].includes(item.href))
  return <><div className="fleet-shell-navigation"><nav aria-label="Main navigation">{top.map(item => item.href === '/inbox' ? <span className="fleet-shell-inbox" data-active={pathname === '/inbox' || undefined} key={item.href}><Icon name="list" /><InboxLink /></span> : <Link key={item.href} to={item.href} activeOptions={{ exact: item.href === '/' }} activeProps={{ 'aria-current': 'page', className: 'fleet-nav-active' }}><Icon name={item.icon} />{item.label}</Link>)}</nav><DestinationSearch key={`${session.person.id}:${eventID ?? ''}`} destinations={destinations} /></div><div className="fleet-shell-context"><span>{admin ? 'Administration' : 'Participant portal'}</span>{event ? <><Link to="/">Switch event</Link><strong>{event.name}</strong><span>{event.stage}</span><span>{event.can_edit ? 'Event editor access' : event.can_read_staff ? 'Staff access' : 'Event access'}</span></> : <span>{eventID ? query.error ? 'Event access unavailable' : 'Checking event access…' : 'No event selected'}</span>}</div></>
}
export function PortalFrame({ account, navigation, children }: { account: ReactNode; navigation?: ReactNode; children: ReactNode }) {
  return <div className="fleet-portal-shell"><a className="skip-link" href="#main">Skip to content</a><header className="fleet-shell-header"><Link className="fleet-shell-brand" to="/" aria-label="Hack in a Box home"><Icon name="layout" />Hack in a Box{import.meta.env.MODE === 'admin' ? ' · Admin' : ''}</Link><div className="fleet-shell-account">{account}</div></header>{navigation}{children}<footer className="fleet-shell-footer">Make something worth sharing.</footer></div>
}
