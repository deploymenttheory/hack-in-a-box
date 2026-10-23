import { describe, expect, it } from 'vitest'
import { permittedDestinations } from './portal-navigation'

describe('current portal destinations', () => {
  it('keeps participant presentation independent of platform creation authority', () => {
    expect(permittedDestinations({ can_create_events: true }, false).map(link => link.href)).not.toContain('/events/new')
    expect(permittedDestinations({ can_create_events: true }, false).map(link => link.href)).toContain('/register')
  })
  it('requires the actual creation fact on the admin surface', () => {
    expect(permittedDestinations({ can_create_events: false }, true).map(link => link.href)).not.toContain('/events/new')
    expect(permittedDestinations({ can_create_events: true }, true).map(link => link.href)).toContain('/events/new')
  })
  it('advertises review only for its current capability independently from staff access', () => {
    const event = { id: 'synthetic-event', name: 'Synthetic event', can_read_staff: true }
    expect(permittedDestinations({ can_create_events: false }, true, event).map(link => link.label)).not.toContain('Support and assessment')
    const mentor = { ...event, can_read_staff: false, can_read_review_coverage: true }
    expect(permittedDestinations({ can_create_events: false }, true, mentor).map(link => link.label)).toContain('Support and assessment')
    expect(permittedDestinations({ can_create_events: false }, true).map(link => link.href).filter(path => path.includes('synthetic-event'))).toEqual([])
  })
})
