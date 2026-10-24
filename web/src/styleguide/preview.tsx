import { GuideSkipLink } from './skip-link'
import { StrictMode, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { Theme } from '../components/theme'
import { brandStyle } from '../design-system/palette'
import { PatternExamples } from '../components/patterns/examples'
import { MediaHistoryExample } from './media-history'
import { MentorOperationsExamples } from './mentor-operations'
import { TeamCoverageExamples } from './team-coverage'
import { ProtectedUploadExample } from './protected-upload'
import { CommunicationExamples } from './communications'
import { LocalRecordExamples } from './local-records'
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
  return <Theme mode={options.mode} density={options.density} style={options.palette === 'platform' ? undefined : brandStyle(palettes[options.palette])}><GuideSkipLink target="preview-main">Skip to examples</GuideSkipLink><div id="preview-main" className="guide-frame-content" tabIndex={-1}><h1>{patterns ? 'Task pattern examples' : 'Component examples'}</h1><p>These interactive examples import the production components and use synthetic local state.</p>{patterns ? <><PatternExamples key={`${options.resetKey}:${options.portal}`} portal={options.portal} withinTheme /><LocalRecordExamples key={'local:' + options.resetKey + ':' + options.portal} /><TeamCoverageExamples key={'coverage:' + options.resetKey + ':' + options.portal} /><ProtectedUploadExample key={'upload:' + options.resetKey + ':' + options.portal} /><CommunicationExamples key={'communications:' + options.resetKey + ':' + options.portal} /><MentorOperationsExamples key={'mentor-operations:' + options.resetKey + ':' + options.portal} /><MediaHistoryExample key={'media-history:' + options.resetKey + ':' + options.portal} /></> : <main><PrimitiveExamples key={options.resetKey} resetKey={options.resetKey} /></main>}</div></Theme>
}
createRoot(document.getElementById('root')!).render(<StrictMode><Preview /></StrictMode>)
