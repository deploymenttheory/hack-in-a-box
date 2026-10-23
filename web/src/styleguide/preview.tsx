import { StrictMode, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { Theme } from '../components/theme'
import { brandStyle } from '../design-system/palette'
import { PrimitiveExamples } from './primitives'
import { isPreviewOptions, palettes, type PreviewOptions } from './preferences'
import '../styles.css'
import './styleguide.css'

function Preview() {
  const [options, setOptions] = useState<PreviewOptions>({ kind: 'guide-preview-options', mode: 'system', palette: 'platform', density: 'comfortable', resetKey: 0 })
  useEffect(() => {
    const receive = (event: MessageEvent<unknown>) => { if (event.origin === location.origin && event.source === parent && isPreviewOptions(event.data)) setOptions(event.data) }
    addEventListener('message', receive)
    parent.postMessage({ kind: 'guide-preview-ready' }, location.origin)
    return () => removeEventListener('message', receive)
  }, [])
  return <Theme mode={options.mode} density={options.density} style={options.palette === 'platform' ? undefined : brandStyle(palettes[options.palette])}><a className="skip-link" href="#preview-main">Skip to examples</a><main id="preview-main" className="guide-frame-content" tabIndex={-1}><h1>Component examples</h1><p>These interactive examples import the production components and use synthetic local state.</p><PrimitiveExamples key={options.resetKey} resetKey={options.resetKey} /></main></Theme>
}
createRoot(document.getElementById('root')!).render(<StrictMode><Preview /></StrictMode>)
