import { useState } from 'react'
import { Button } from '../components/ui/button'

export function Snippet({ children }: { children: string }) {
  const [message, setMessage] = useState('')
  async function copy() {
    try { await navigator.clipboard.writeText(children); setMessage('Code copied.') }
    catch { setMessage('Select and copy the code below.') }
  }
  return <div className="guide-snippet"><div className="guide-snippet-toolbar"><span>Usage</span><Button icon="code" variant="ghost" size="sm" onClick={() => void copy()}>Copy code</Button><span role="status">{message}</span></div><pre><code>{children}</code></pre></div>
}
