import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, privateKey, unwrap } from './api/client'
import { useIdentity } from './session'
import { Button } from './components/ui/button'
import { Failure, Loading } from './components/states'

export function GitHubAccount() {
  const identity = useIdentity()
  const [acknowledged, setAcknowledged] = useState(false)
  const [pending, setPending] = useState(false)
  const [failure, setFailure] = useState<Error | null>(null)
  const query = useQuery({ queryKey: privateKey(identity.person.id, 'account', 'github'), queryFn: async () => unwrap(await api.GET('/api/v1/account/github')), refetchOnWindowFocus: 'always' })
  const callbackFailed = new URLSearchParams(window.location.search).get('github') === 'failed'
  const act = async (unlink: boolean) => {
    if (!query.data || !acknowledged) return
    setPending(true); setFailure(null)
    const params = { header: { 'X-CSRF-Token': identity.csrf_token, 'If-Match': `"${query.data.revision}"` } }
    const body = { acknowledge_invitation_impact: acknowledged }
    try {
      if (unlink) { unwrap(await api.DELETE('/api/v1/account/github', { params, body })); setAcknowledged(false); await query.refetch() }
      else { const result = unwrap(await api.POST('/api/v1/account/github/authorize', { params, body })); window.location.assign(result.url) }
    } catch (error) { setFailure(error instanceof Error ? error : new Error('GitHub linking failed. Please try again.')) }
    finally { setPending(false) }
  }
  return <section className="panel"><h2>GitHub account</h2><p>Verify the GitHub account you control so repository invitations reach the right person. Your organisation sign-in stays separate.</p>
    {query.isPending ? <Loading /> : query.error ? <Failure error={query.error} retry={() => void query.refetch()} /> : <>
      {callbackFailed && <p role="alert">GitHub verification did not complete. Your existing link has been kept. Start again to verify your account.</p>}
      {query.data.account ? <p><strong>@{query.data.account.login}</strong> · Verified GitHub ID {query.data.account.github_id}</p> : <p>No verified GitHub account linked.</p>}
      {!query.data.enabled && <p>GitHub linking is not configured for this installation.</p>}
      {query.data.invitations_needing_review > 0 && <p role="status">{query.data.invitations_needing_review} pending invitation(s) need organiser review.</p>}
      {(query.data.enabled || query.data.account) && <><label className="check"><input type="checkbox" checked={acknowledged} onChange={event => setAcknowledged(event.target.checked)} />I understand that changing or unlinking my GitHub account puts pending invitations on hold for organiser review. Existing repository access is not transferred or removed.</label><div className="actions"><Button disabled={!query.data.enabled || !acknowledged || pending} onClick={() => void act(false)}>{pending ? 'Working…' : query.data.account ? 'Verify or change GitHub account' : 'Link GitHub account'}</Button>{query.data.account && <Button disabled={!acknowledged || pending} onClick={() => void act(true)}>Unlink GitHub account</Button>}</div></>}
      {failure && <Failure error={failure} />}
    </>}
  </section>
}
