import { cleanup, fireEvent, render, screen, act } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { useRef } from 'react'
import { CommunicationSafety, useCommunicationCommands, useCommunicationCommandNamespace } from './communication-tests'
import { TodayActionSafety } from './today-safety'
import { IdentityContext } from './session'
import { planningDepartureAllowed } from './planning-operations'
import { person, firstID, secondID } from '../e2e/fixtures'
vi.mock('@tanstack/react-router', () => ({ useBlocker: () => undefined }))
afterEach(() => { cleanup(); vi.restoreAllMocks() })
const draft = 'synthetic-safe-session', prefix = `${person.id}:${draft}:${firstID}`
function Commands() {
 const prep = useCommunicationCommands(`${prefix}:preparation:current-task`), ack = useCommunicationCommands(`${prefix}:resource-acknowledgement:current-resource`), otherEvent = useCommunicationCommands(`${person.id}:${draft}:${secondID}:preparation:other-task`), repository = useCommunicationCommands(`${prefix}:repository:other-operation`), otherSession = useCommunicationCommands(`${person.id}:different-session:${firstID}:preparation:other-task`), ids = useRef<string[]>([])
 const record = (operation: string) => ({ operation, revision: 7, command: { action: 'update' } })
 return <><button onClick={() => { ids.current = [prep.begin(record('Owned preparation')), ack.begin(record('Owned reading'))]; otherEvent.begin(record('OTHER EVENT')); repository.begin(record('OTHER REPOSITORY')); otherSession.begin(record('OTHER SESSION')) }}>Begin records</button><button onClick={() => { prep.uncertain(ids.current[0]); ack.uncertain(ids.current[1]) }}>Original responses lost</button><output data-testid="unrelated">{JSON.stringify([otherEvent.attempts, repository.attempts, otherSession.attempts])}</output></>
}
function Namespace() { const items = useCommunicationCommandNamespace(`${prefix}:preparation`); return <output data-testid="exact-preparation">{JSON.stringify(items)}</output> }
test('the safe observer owns only exact actor/session/event preparation and reading namespaces, preserving unrelated history and unknown barriers', () => {
 render(<CommunicationSafety><IdentityContext.Provider value={{ person, draft_scope: draft, csrf_token: 'synthetic', can_create_events: false }}><TodayActionSafety eventID={firstID}><Commands /><Namespace /></TodayActionSafety></IdentityContext.Provider></CommunicationSafety>)
 fireEvent.click(screen.getByRole('button', { name: 'Begin records' }))
 const history = screen.getByRole('complementary', { name: 'Unconfirmed command history' })
 expect(history).toHaveTextContent('Owned preparation'); expect(history).toHaveTextContent('Owned reading')
 expect(history).not.toHaveTextContent('OTHER EVENT'); expect(history).not.toHaveTextContent('OTHER REPOSITORY'); expect(history).not.toHaveTextContent('OTHER SESSION')
 expect(screen.getByTestId('exact-preparation')).not.toHaveTextContent('Owned reading')
 const unrelated = screen.getByTestId('unrelated').textContent
 expect(planningDepartureAllowed()).toBe(false)
 fireEvent.click(screen.getByRole('button', { name: 'Original responses lost' }))
 const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
 act(() => { expect(planningDepartureAllowed()).toBe(false) })
 confirm.mockReturnValue(true)
 act(() => { expect(planningDepartureAllowed()).toBe(true) })
 expect(history).toHaveTextContent('Original outcome remains unconfirmed')
 expect(screen.getByTestId('unrelated').textContent).toBe(unrelated)
})
