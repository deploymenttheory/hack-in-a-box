import { useCallback, useEffect, useRef } from 'react'
import type { PreviewOptions } from './preferences'

export function PreviewFrame({ options, width, story = 'components' }: { options: PreviewOptions; width: string; story?: 'components' | 'patterns' }) {
  const frame = useRef<HTMLIFrameElement>(null)
  const send = useCallback(() => frame.current?.contentWindow?.postMessage(options, location.origin), [options])
  useEffect(() => { send() }, [send])
  useEffect(() => {
    const ready = (event: MessageEvent<unknown>) => {
      if (event.origin !== location.origin || event.source !== frame.current?.contentWindow || !event.data || typeof event.data !== 'object' || !('kind' in event.data) || Object.keys(event.data).length !== 1) return
      if (event.data.kind === 'guide-preview-ready') send()
      // The real nested viewport must fit before an overlay locks ancestor
      // scrolling. Only our own public frame may request this geometry action.
      if (event.data.kind === 'guide-preview-focus') frame.current?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    }
    addEventListener('message', ready)
    return () => removeEventListener('message', ready)
  }, [send])
  return <div className="guide-frame-host" style={{ maxInlineSize: width === 'full' ? undefined : `${width}px` }}><iframe ref={frame} src={story === 'patterns' ? '/patterns.html' : '/samples.html'} title={story === 'patterns' ? 'Production task pattern examples' : 'Production component examples'} className="guide-example-frame" onLoad={send} allow="clipboard-write" /><p className="guide-frame-caption">A real nested viewport. Theme and density changes preserve example state; Reset examples clears it.</p></div>
}
