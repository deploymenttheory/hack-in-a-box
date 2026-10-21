import { createContext, useContext } from 'react'
import type { Session } from './api/client'
export const IdentityContext = createContext<Session | null>(null)
export function useIdentity() {
  const identity = useContext(IdentityContext)
  if (!identity) throw new Error('An authenticated workspace is required.')
  return identity
}
