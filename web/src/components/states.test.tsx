import { render, screen } from '@testing-library/react'
import { Failure } from './states'
import { APIError, privateKey } from '../api/client'
import { Theme } from './theme'
import { QueryClient } from '@tanstack/react-query'
import { test, expect } from 'vitest'

test('errors never render raw server details', () => {
  render(<Failure error={new Error('token=secret database=private')} />)
  expect(screen.getByRole('alert')).toHaveTextContent('Please try again')
  expect(screen.queryByText(/token=secret/)).not.toBeInTheDocument()
})
test('expired sessions have a useful action', () => {
  render(<Failure error={new APIError(401, 'unauthenticated')} />)
  expect(screen.getByRole('alert')).toHaveTextContent('Sign in to continue')
})
test('permission denial explains why access is unavailable', () => {
  render(<Failure error={new APIError(403, 'forbidden')} />)
  expect(screen.getByRole('alert')).toHaveTextContent('You do not have access to this resource.')
})
test('private cache keys separate both accounts and events', () => {
  const cache = new QueryClient()
  cache.setQueryData(privateKey('person-a', 'event-a', 'team'), 'private')
  expect(cache.getQueryData(privateKey('person-b', 'event-a', 'team'))).toBeUndefined()
  expect(cache.getQueryData(privateKey('person-a', 'event-b', 'team'))).toBeUndefined()
  cache.clear()
  expect(cache.getQueryData(privateKey('person-a', 'event-a', 'team'))).toBeUndefined()
})
test('portal host remains in its event theme', () => {
  const { container } = render(<Theme accent="#123456"><p>Workspace</p></Theme>)
  expect(container.querySelector('[data-theme-portals]')?.closest('.theme')).toHaveStyle('--primary: #123456')
})
