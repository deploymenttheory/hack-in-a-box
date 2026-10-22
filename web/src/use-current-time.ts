import { useEffect, useState } from 'react'

// Display hints follow the browser clock; server permissions remain authoritative.
export function useCurrentTime() {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => { const timer = window.setInterval(() => setNow(Date.now()), 15_000); return () => window.clearInterval(timer) }, [])
  return now
}
