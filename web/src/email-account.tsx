import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'

function usePrivateField() {
  const input = useRef<HTMLInputElement>(null)
  const [hasValue, setHasValue] = useState(false)
  const clear = () => { if (input.current) input.current.value = ''; setHasValue(false) }
  useEffect(() => {
    const wipe = () => { if (input.current) input.current.value = ''; setHasValue(false) }
    const hide = () => { if (document.visibilityState !== 'visible') wipe() }
    window.addEventListener('pagehide', wipe); document.addEventListener('visibilitychange', hide)
    return () => { window.removeEventListener('pagehide', wipe); document.removeEventListener('visibilitychange', hide) }
  }, [])
  const attach = useCallback((element: HTMLInputElement | null) => { if (input.current && input.current !== element) input.current.value = ''; input.current = element }, [])
  return { input, attach, hasValue, clear, change: (event: React.ChangeEvent<HTMLInputElement>) => setHasValue(event.currentTarget.value.length > 0) }
}

export function EmailAccount() {
  const identity = useIdentity()
  const { input: addressInput, attach: attachAddress, hasValue: hasAddress, clear: clearAddress, change: addressChanged } = usePrivateField()
  const { input: codeInput, attach: attachCode, hasValue: hasCode, clear: clearCode, change: codeChanged } = usePrivateField()
  const [pending, setPending] = useState(false)
  const [failure, setFailure] = useState<Error | null>(null)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => { const timer = window.setInterval(() => setNow(Date.now()), 1000); return () => window.clearInterval(timer) }, [])
  const query = useQuery({ queryKey: privateKey(identity.person.id, 'account', 'email'), queryFn: async () => unwrap(await api.GET('/api/v1/account/email')), refetchOnWindowFocus: 'always' })
  const status = query.data
  const cooldown = status ? Math.max(0, Math.ceil((query.dataUpdatedAt + status.retry_after_seconds * 1000 - now) / 1000)) : 0
  const act = async (confirm: boolean) => {
    const value = confirm ? codeInput.current?.value ?? '' : addressInput.current?.value.trim() ?? ''
    clearAddress(); clearCode(); setPending(true); setFailure(null)
    try {
      const params = { header: { 'X-CSRF-Token': identity.csrf_token } }
      if (confirm) unwrap(await api.POST('/api/v1/account/email/confirmation', { params, body: { code: value } }))
      else unwrap(await api.POST('/api/v1/account/email/verification', { params, body: { email: value } }))
      await query.refetch()
    } catch (error) { setFailure(error instanceof Error ? error : new Error('Email confirmation did not complete.')); await query.refetch() }
    finally { setPending(false) }
  }
  return <section className="panel"><h2>Personal email</h2><p>Confirm an address for individual event messages. You continue signing in with your organisation account.</p>
    {query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <>
      {status?.verified ? <p><strong>{status.masked_mailbox}</strong> · {status.provenance === 'code_confirmed' ? 'Confirmed by you' : 'Verified by your sign-in provider'}</p> : <p>No confirmed email address.</p>}
      {!status?.configured && <p>Personal email is not configured for this installation.</p>}
      {status?.configured && <form autoComplete="off" onSubmit={event => { event.preventDefault(); void act(false) }}><label htmlFor="personal-email-address">{status.verified ? 'New email address' : 'Email address'}</label><input ref={attachAddress} id="personal-email-address" type="email" autoComplete="off" maxLength={254} required onChange={addressChanged} /><Button type="submit" disabled={pending || !hasAddress || cooldown > 0}>Send confirmation code</Button>{cooldown > 0 && <p role="status">You can request another code in {cooldown} seconds.</p>}</form>}
      {status?.pending && <><p role="status">{status.request_state === 'accepted' ? 'The email relay accepted your confirmation request. Check your inbox for the code.' : status.request_state === 'ambiguous' ? 'Confirmation delivery is uncertain. If a code arrives, you can still use it.' : status.request_state === 'prepared' ? 'Your request was recorded. Delivery has not been confirmed.' : 'The email relay did not accept your confirmation request.'}</p><p>A code expires after 10 minutes. Requesting a new code replaces the previous one.</p><form autoComplete="off" onSubmit={event => { event.preventDefault(); void act(true) }}><label htmlFor="personal-email-code">Eight-digit confirmation code</label><input ref={attachCode} id="personal-email-code" type="password" inputMode="numeric" pattern={'[0-9]{8}'} minLength={8} maxLength={8} autoComplete="off" required onChange={codeChanged} /><Button type="submit" disabled={pending || !hasCode}>Confirm email address</Button></form></>}
      {failure && <Failure error={failure} />}
    </>}
  </section>
}
