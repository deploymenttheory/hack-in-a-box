import { StrictMode, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { Theme } from '../components/theme'
import { brandStyle } from '../design-system/palette'
import { PatternExamples } from '../components/patterns/examples'
import { PrimitiveExamples } from './primitives'
import { isPreviewOptions, palettes, type PreviewOptions } from './preferences'
import './production.css'
import './styleguide.css'

function Preview() {
  const [options, setOptions] = useState<PreviewOptions>({ kind: 'guide-preview-options', mode: 'system', palette: 'platform', density: 'comfortable', resetKey: 0, portal: 'admin' })
  useEffect(() => {
    const receive = (event: MessageEvent<unknown>) => { if (event.origin === location.origin && event.source === parent && isPreviewOptions(event.data)) setOptions(event.data) }
    addEventListener('message', receive)
    parent.postMessage({ kind: 'guide-preview-ready' }, location.origin)
    return () => removeEventListener('message', receive)
  }, [])
  const patterns = location.pathname === '/patterns.html'
  return <Theme mode={options.mode} density={options.density} style={options.palette === 'platform' ? undefined : brandStyle(palettes[options.palette])}><a className="skip-link" href="#preview-main">Skip to examples</a><div id="preview-main" className="guide-frame-content" tabIndex={-1}><h1>{patterns ? 'Task pattern examples' : 'Component examples'}</h1><p>These interactive examples import the production components and use synthetic local state.</p>{patterns ? <PatternExamples key={`${options.resetKey}:${options.portal}`} portal={options.portal} withinTheme /> : <main><PrimitiveExamples key={options.resetKey} resetKey={options.resetKey} /></main>}</div></Theme>
}
createRoot(document.getElementById('root')!).render(<StrictMode><Preview /></StrictMode>)
