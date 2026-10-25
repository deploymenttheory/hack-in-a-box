import { ResultsExample } from './results'
import { QualificationExample } from './qualification'
import { JudgingExample } from './judging'
import { SubmissionFreezeExample } from './submission-freeze'
import { SessionWindowExample } from './session-windows'
import { SessionBookingExample } from './sessions'
import { RepositoryExamples } from './repositories'
import { TodayExample } from './today'
import { OutcomeExamples } from './outcomes'
import { GuideSkipLink } from './skip-link'
import { StrictMode, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { Theme } from '../components/theme'
import { brandStyle } from '../design-system/palette'
import { PatternExamples } from '../components/patterns/examples'
import { PresentationExamples, PresentationDecisionExamples } from './presentation'
import { MediaLifecycleExamples } from './media-lifecycle'
import { MediaHistoryExample } from './media-history'
import { MentorOperationsExamples } from './mentor-operations'
import { TeamCoverageExamples } from './team-coverage'
import { ProtectedUploadExample } from './protected-upload'
import { CommunicationExamples } from './communications'
import { SurveyWorkflowExamples } from './surveys'
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
    let pointerActive = false
    const beginPointer = () => { pointerActive = true }
    const endPointer = () => { pointerActive = false }
    const focusPreview = (event: Event) => {
      // Moving the ancestor viewport between pointer down and release cancels
      // the intended click. Keyboard focus may align immediately; pointer
      // activation aligns only after the target received its actual click.
      if (event.type === 'focusin' && pointerActive) return
      if (parent !== window && event.target instanceof Element && event.target.closest('button,input,select,textarea,a,[tabindex]:not([tabindex="-1"])')) parent.postMessage({ kind: 'guide-preview-focus' }, location.origin)
    }
    addEventListener('focusin', focusPreview)
    addEventListener('pointerdown', beginPointer)
    addEventListener('pointerup', endPointer)
    addEventListener('pointercancel', endPointer)
    addEventListener('blur', endPointer)
    addEventListener('click', focusPreview)
    parent.postMessage({ kind: 'guide-preview-ready' }, location.origin)
    return () => { removeEventListener('message', receive); removeEventListener('focusin', focusPreview); removeEventListener('pointerdown', beginPointer); removeEventListener('pointerup', endPointer); removeEventListener('pointercancel', endPointer); removeEventListener('blur', endPointer); removeEventListener('click', focusPreview) }
  }, [])
  const patterns = location.pathname === '/patterns.html'
  return <Theme mode={options.mode} density={options.density} style={options.palette === 'platform' ? undefined : brandStyle(palettes[options.palette])}><GuideSkipLink target="preview-main">Skip to examples</GuideSkipLink><div id="preview-main" className="guide-frame-content" tabIndex={-1}><h1>{patterns ? 'Task pattern examples' : 'Component examples'}</h1><p>These interactive examples import the production components and use synthetic local state.</p>{patterns ? <><PatternExamples key={`${options.resetKey}:${options.portal}`} portal={options.portal} withinTheme /><LocalRecordExamples key={'local:' + options.resetKey + ':' + options.portal} /><TeamCoverageExamples key={'coverage:' + options.resetKey + ':' + options.portal} /><ProtectedUploadExample key={'upload:' + options.resetKey + ':' + options.portal} /><ResultsExample key={'results:' + options.resetKey + ':' + options.portal}/><QualificationExample key={'qualification:' + options.resetKey + ':' + options.portal}/><SubmissionFreezeExample key={'freeze:' + options.resetKey + ':' + options.portal}/><RepositoryExamples key={'repositories:' + options.resetKey + ':' + options.portal} /><OutcomeExamples key={'outcomes:' + options.resetKey + ':' + options.portal} /><CommunicationExamples key={'communications:' + options.resetKey + ':' + options.portal} /><MentorOperationsExamples key={'mentor-operations:' + options.resetKey + ':' + options.portal} /><MediaHistoryExample key={'media-history:' + options.resetKey + ':' + options.portal} /><JudgingExample key={'judging:' + options.resetKey + ':' + options.portal} /><SessionBookingExample key={'sessions:' + options.resetKey + ':' + options.portal} /><SessionWindowExample key={'session-windows:' + options.resetKey + ':' + options.portal} /><TodayExample key={'today:' + options.resetKey + ':' + options.portal} /><SurveyWorkflowExamples key={'surveys:' + options.resetKey + ':' + options.portal} /><PresentationDecisionExamples key={'presentation-decision:' + options.resetKey + ':' + options.portal}/><PresentationExamples key={'presentation:' + options.resetKey + ':' + options.portal}/><MediaLifecycleExamples key={'media-lifecycle:' + options.resetKey + ':' + options.portal} /></> : <main><PrimitiveExamples key={options.resetKey} resetKey={options.resetKey} /></main>}</div></Theme>
}
createRoot(document.getElementById('root')!).render(<StrictMode><Preview /></StrictMode>)
