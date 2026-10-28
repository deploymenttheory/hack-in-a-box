import { useLayoutEffect, useRef, useState } from 'react'
import { Button } from '../components/ui/button'
import { focusFirstInvalid } from '../components/ui/fields'
import { assignmentProblems, roundCreationProblems } from '../judging-setup-model'
import { emptyJudgingSetupInput, JudgingSetupCurrent, JudgingSetupDecision, JudgingSetupFields, type JudgingSetupReview } from '../judging-setup-view'
import { judgingSetupGuideSource } from './judging-setup-fixture'
import { Snippet } from './snippet'

export function JudgingSetupExample() {
  const [source, setSource] = useState(() => judgingSetupGuideSource()), [input, setInput] = useState(emptyJudgingSetupInput)
  const [errors, setErrors] = useState<Record<string, string>>({}), [review, setReview] = useState<JudgingSetupReview>(), [notice, setNotice] = useState('')
  const form = useRef<HTMLFormElement>(null), opener = useRef<HTMLElement | null>(null)
  useLayoutEffect(() => { if (form.current && Object.keys(errors).length) focusFirstInvalid(form.current) }, [errors])
  function reset(withRound: boolean) { setSource(judgingSetupGuideSource(withRound)); setInput(emptyJudgingSetupInput()); setErrors({}); setReview(undefined); setNotice('') }
  function begin(button: HTMLElement) {
    const invalid = source.run ? assignmentProblems(source, input.teamID, input.judgeIDs, input.reason, input.acknowledgeGap) : roundCreationProblems(source, input.rubric, input.reason)
    setErrors(invalid); if (Object.keys(invalid).length) return
    opener.current = button; setReview({ kind: source.run ? 'assign' : 'create', source: structuredClone(source), input: structuredClone(input) })
  }
  return <section aria-label="Final judging setup example" className="guide-section"><h2>Plan final judging</h2><p>Production fields and captured review with a synthetic local owner. This example sends no requests, creates no native receipts and grants no scoring or assignment authority.</p><div className="fleet-pattern-actions"><Button icon="refresh" variant="outline" onClick={() => reset(false)}>Show unconfigured synthetic round</Button><Button icon="user" variant="outline" onClick={() => reset(true)}>Show existing synthetic round</Button></div><JudgingSetupCurrent source={source} /><div hidden={!!review}><JudgingSetupFields source={source} input={input} change={setInput} errors={errors} disabled={false} review={begin} form={form} /></div>{review && <JudgingSetupDecision value={review} pending={false} disabled={false} cancel={() => setReview(undefined)} confirm={() => { setReview(undefined); setNotice('Local review finished. No round or assignment was sent.') }} returnFocus={() => opener.current} />}<p role="status" aria-label="Local judging setup result">{notice}</p><p>Require explicit rubric selection and named current judges. Show missing judge places in text; an intentionally incomplete assignment needs its own acknowledgement. Invalid input reveals and focuses the first field. Cancelling review returns focus to its initiating control. Production binds current authority, source, team and revision again before sending; unknown changes retain safe original metadata and are never replayed or inferred from a current read.</p><Snippet>{'<JudgingSetupFields source={currentAuthorisedSetup}\n  input={localInput} change={setInput} errors={errors}\n  disabled={pausedOrUnconfirmed} review={captureExactReview} form={form} />\n// Use JudgingSetupDecision for explicit confirmation.\n// The native owner checks current source and exact original receipts.\n// Real scoring starts only through the existing native assessment gates.'}</Snippet></section>
}
