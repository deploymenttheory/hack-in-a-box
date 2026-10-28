import { useRef, useState } from 'react'
import { ChallengeRubricChoice } from '../challenge-rubric-choice'
import { publicChallengeRubric, type RubricReference } from '../challenge-rubric-model'
import { rubricGuideConfiguration } from './rubric-fixture'
import { Button } from '../components/ui/button'
import { Snippet } from './snippet'

export function ChallengeRubricExample() {
  const initial = rubricGuideConfiguration(), original = publicChallengeRubric({ ...initial, reference: { ...initial.reference, version: 1, digest: 'a'.repeat(64) }, approved: true })
  const [approved, setApproved] = useState(original), [selected, setSelected] = useState<typeof original>(), [paused, setPaused] = useState(false), [notice, setNotice] = useState('')
  const advance = useRef<HTMLButtonElement>(null)
  function choose(reference?: RubricReference) { setSelected(reference ? structuredClone(approved) : undefined) }
  return <section className="guide-section" aria-label="Challenge rubric binding example"><h2>Bind an exact approved rubric</h2><p>The production selector uses synthetic local snapshots here. It makes no application requests or publication decisions.</p>
    <ChallengeRubricChoice data={{ approved, selected }} value={selected?.reference} paused={paused} disabled={false} choose={choose} configure={() => setNotice('In the administrator portal this opens judging criteria through the unsaved-content navigation guard.')} />
    <div className="fleet-pattern-actions"><Button ref={advance} type="button" variant="outline" icon="refresh" onClick={() => { setApproved({ ...original, reference: { ...original.reference, version: approved.reference.version + 1, digest: 'b'.repeat(64) } }); requestAnimationFrame(() => advance.current?.focus()) }}>Simulate a newer approval</Button><Button type="button" variant="outline" onClick={() => setPaused(!paused)}>{paused ? 'Restore criteria reads' : 'Pause criteria reads'}</Button></div>
    {notice && <p role="status">{notice}</p>}<p>Keep the saved tuple unchanged when a newer version appears. Name the current and selected versions, show public bounds, weights and anchors, and require a deliberate choice before saving. Pending reads preserve keyboard position and unsent text while decisions pause. A definite denial clears the owning private editor. Never place private judge guidance in a challenge document or attendee preview; current reads cannot resolve an unknown write.</p>
    <Snippet>{'<ChallengeRubricChoice data={{ approved, selected }}\n  value={document.rubric} paused={readPendingOrFailed}\n  disabled={mutationPendingOrUnknown}\n  choose={reference => changeDocument({ ...document, rubric: reference })}\n  configure={openJudgingCriteria} />\n// Save only the exact reference. Fresh native authority and approval\n// preflights belong to the owner, before any versioned content write.'}</Snippet>
  </section>
}
