import { useLayoutEffect, useRef, useState } from 'react'
import { Button } from '../components/ui/button'
import { focusFirstInvalid } from '../components/ui/fields'
import { checkpointProblems, checkpointWindowProblems, mentorAssessmentRuns, mentorRunProblems } from '../mentor-checkpoint-planning-model'
import { editedMentorPlannerErrors, emptyMentorPlannerInput, MentorPlannerCurrent, MentorPlannerDecision, MentorPlannerFields, type MentorPlannerReview } from '../mentor-checkpoint-planning-view'
import { mentorCheckpointGuideSource } from './mentor-checkpoint-fixture'
import { Snippet } from './snippet'

export function MentorCheckpointPlanningExample() {
  const [source, setSource] = useState(() => mentorCheckpointGuideSource()), [input, setInput] = useState(() => emptyMentorPlannerInput(source))
  const [errors, setErrors] = useState<Record<string, string>>({}), [review, setReview] = useState<MentorPlannerReview>(), [notice, setNotice] = useState('')
  const form = useRef<HTMLFormElement>(null), opener = useRef<HTMLElement | null>(null), focusValidation = useRef(false)
  useLayoutEffect(() => { if (focusValidation.current) { focusValidation.current = false; if (form.current) focusFirstInvalid(form.current) } }, [errors])
  function showIssues(issues: Record<string, string>) { focusValidation.current = Object.keys(issues).length > 0; setErrors(issues) }
  function reset(withRun: boolean) { const next = mentorCheckpointGuideSource(withRun); setSource(next); setInput(emptyMentorPlannerInput(next)); setErrors({}); setReview(undefined); setNotice('') }
  function begin(button: HTMLElement) {
    const invalid = input.task === 'run' ? mentorRunProblems(source, input.purpose, input.rubric, input.reason) : checkpointProblems(source, input.checkpoint)
    showIssues(invalid); if (Object.keys(invalid).length) return
    opener.current = button
    if (input.task === 'run') setReview({ kind: 'run', source: structuredClone(source), input: structuredClone(input) })
    else {
      const run = mentorAssessmentRuns(source).find(value => value.id === input.checkpoint.runID)!
      // This guide uses an explicit synthetic UTC fixture only. Production
      // resolves event-local clocks and DST through the native time API.
      const window = { opens: new Date(input.checkpoint.opens + 'Z').toISOString(), closes: new Date(input.checkpoint.closes + 'Z').toISOString() }
      const issues = checkpointWindowProblems(source.event, run.purpose as 'checkpoint' | 'mentor_final', window)
      showIssues(issues); if (Object.keys(issues).length) return
      setReview({ kind: 'checkpoint', source: structuredClone(source), intent: { run, criteria: source.approved!.definition, window, timezone: 'UTC', command: { version: 0, reason: input.checkpoint.reason, definition: { id: '66666666-6666-4666-8666-666666666666', name: input.checkpoint.name, run_id: run.id, opens_wall_time: input.checkpoint.opens, closes_wall_time: input.checkpoint.closes, dst_choice: input.checkpoint.dst } } } })
    }
  }
  return <section aria-label="Mentor checkpoint planning example" className="guide-section" onClickCapture={event => { const link = (event.target as HTMLElement).closest('a'); if (link?.getAttribute('href')?.startsWith('/events/')) { event.preventDefault(); setNotice(`Production handoff: ${link.textContent}. This local example stays in the guide.`) } }}><h2>Plan mentor checkpoints</h2><p>Production fields and captured review with local synthetic UTC data. This example sends no requests and creates no assessment, checkpoint or native acknowledgement.</p><div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" onClick={() => reset(false)}>Show assessment creation example</Button><Button icon="calendar" variant="outline" onClick={() => reset(true)}>Show checkpoint window example</Button></div><div hidden={!!review}><MentorPlannerFields source={source} input={input} change={next => { setErrors(editedMentorPlannerErrors(input, next, errors)); setInput(next) }} errors={errors} disabled={false} review={begin} form={form} /></div>{review && <MentorPlannerDecision value={review} pending={false} disabled={false} cancel={() => setReview(undefined)} confirm={() => { setReview(undefined); setNotice('Local review finished. No assessment or checkpoint was sent.') }} returnFocus={() => opener.current} />}<MentorPlannerCurrent source={source} /><p role="status" aria-label="Local mentor planning result">{notice}</p><p>Keep live observations and frozen-output qualification separate. Show the resolved window and event timezone before confirmation. Field errors describe their own controls; focus the first invalid field, then return focus to the initiating control on cancellation. Production resolves local clock ambiguity and current authority/version again; current reads never acknowledge unknown original writes.</p><Snippet>{'<MentorPlannerFields source={currentAuthorisedPlanning}\n  input={input} change={setInput} errors={fieldErrors}\n  disabled={pausedOrUnconfirmed} review={resolveNativeWindowAndCapture}\n  form={form} />\n// MentorPlannerDecision confirms the captured source/window.\n// Native APIs enforce current organiser, source and cutoff gates.\n// Keep original unknown outcomes and safe metadata across routes.'}</Snippet></section>
}
