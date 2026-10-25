import { fireEvent, render, renderHook, screen, act } from '@testing-library/react'
import { useRef, useState } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { useCoverageFocus } from './coverage-navigation'
import { CoverageDetail } from './coverage-detail'

let frames: Map<number, FrameRequestCallback>
beforeEach(() => {
  frames = new Map(); let nextFrame = 0
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { frames.set(++nextFrame, callback); return nextFrame })
  vi.stubGlobal('cancelAnimationFrame', (frame: number) => frames.delete(frame))
  vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue([{} as DOMRect] as unknown as DOMRectList)
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })
function paint() { act(() => { const callbacks = [...frames.values()]; frames.clear(); for (const callback of callbacks) callback(0) }) }

function ClosingWorkspace({ disabled = false, duplicate = false, move = false, detail = false, guarded = false }: { disabled?: boolean; duplicate?: boolean; move?: boolean; detail?: boolean; guarded?: boolean }) {
  const root = useRef<HTMLElement>(null), elsewhere = useRef<HTMLButtonElement>(null), focus = useCoverageFocus(root)
  const [editing, setEditing] = useState(true)
  const source = <><button key="source" disabled={disabled}>Edit capacity</button>{duplicate && <button>Edit capacity</button>}</>
  return <><button ref={elsewhere}>Another destination</button><section ref={root} tabIndex={-1} aria-label="Current coverage workspace">{editing ? <button key="confirm" onClick={() => {
    setEditing(false)
    focus.restore({ button: 'Edit capacity' }, move || guarded)
    if (move) elsewhere.current?.focus()
  }}>Confirm capacity</button> : detail ? <CoverageDetail title="Returned roster" close={() => undefined}>{source}</CoverageDetail> : source}</section></>
}

test('restoration resolves the newly committed source instead of its earlier detached control', () => {
  render(<ClosingWorkspace />)
  const confirm = screen.getByRole('button', { name: 'Confirm capacity' })
  confirm.focus(); fireEvent.click(confirm)
  expect(confirm.isConnected).toBe(false)
  paint()
  expect(screen.getByRole('button', { name: 'Edit capacity' })).toHaveFocus()
})

test.each([{ disabled: true }, { duplicate: true }])('an unavailable or ambiguous current source falls back to its current workspace (%j)', props => {
  render(<ClosingWorkspace {...props} />)
  fireEvent.click(screen.getByRole('button', { name: 'Confirm capacity' }))
  paint()
  expect(screen.getByRole('region', { name: 'Current coverage workspace' })).toHaveFocus()
})

test('a deliberate focus move before the closing commit wins over guarded restoration', () => {
  render(<ClosingWorkspace move />)
  const confirm = screen.getByRole('button', { name: 'Confirm capacity' })
  confirm.focus(); fireEvent.click(confirm)
  paint()
  expect(screen.getByRole('button', { name: 'Another destination' })).toHaveFocus()
})

test('an unmounted owner cannot use a retained restoration closure to focus a replacement workspace', () => {
  const initial = document.createElement('section'), replacement = document.createElement('section')
  const button = document.createElement('button'); button.textContent = 'Edit capacity'; replacement.append(button)
  document.body.append(initial, replacement)
  const root = { current: initial as HTMLElement | null }
  const { result, unmount } = renderHook(() => useCoverageFocus(root))
  const restore = result.current.restore
  unmount(); root.current = replacement
  const focus = vi.spyOn(button, 'focus')
  act(() => restore({ button: 'Edit capacity' }))
  paint()
  expect(focus).not.toHaveBeenCalled()
  initial.remove(); replacement.remove()
})

test('restoration follows the returned inspector heading autofocus', () => {
  render(<ClosingWorkspace detail />)
  const confirm = screen.getByRole('button', { name: 'Confirm capacity' })
  confirm.focus(); fireEvent.click(confirm)
  paint()
  expect(screen.getByRole('button', { name: 'Edit capacity' })).toHaveFocus()
})

test('teardown cancels a committed pending restoration frame', () => {
  const initial = document.createElement('section'), replacement = document.createElement('section')
  const button = document.createElement('button'); button.textContent = 'Edit capacity'; replacement.append(button)
  document.body.append(initial, replacement)
  const root = { current: initial as HTMLElement | null }
  const { result, unmount } = renderHook(() => useCoverageFocus(root))
  act(() => result.current.restore({ button: 'Edit capacity' }))
  expect(frames.size).toBe(1)
  unmount(); root.current = replacement
  expect(frames.size).toBe(0)
  const focus = vi.spyOn(button, 'focus'); paint()
  expect(focus).not.toHaveBeenCalled()
  initial.remove(); replacement.remove()
})

test('a deliberate focus move after commit wins before its restoration frame', () => {
  render(<ClosingWorkspace guarded />)
  const confirm = screen.getByRole('button', { name: 'Confirm capacity' })
  confirm.focus(); fireEvent.click(confirm)
  const destination = screen.getByRole('button', { name: 'Another destination' })
  destination.focus(); paint()
  expect(destination).toHaveFocus()
})
