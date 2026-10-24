import type { PrivateRequestScope } from './private-request-scope'

/** Query functions can start before layout effects renew a newly mounted scope. */
export async function communicationRead<T>(scope: PrivateRequestScope, read: (signal: AbortSignal) => Promise<T>): Promise<T> {
  await Promise.resolve()
  const ticket = scope.capture()
  ticket.assert()
  const result = await read(ticket.signal)
  ticket.assert()
  return result
}
