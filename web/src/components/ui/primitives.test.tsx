import { useRef, useState } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'
import { useForm } from 'react-hook-form'
import { Theme } from '../theme'
import { Button, IconButton, ActionLink } from './button'
import { Field, Input, Select, Checkbox, Textarea, focusFirstInvalid } from './fields'
import { Dialog, DialogTrigger, DialogContent, DialogClose, ConfirmationDialog } from './dialog'
import { Menu, MenuTrigger, MenuContent, MenuItem } from './menu'
import { Popover, PopoverTrigger, PopoverContent } from './popover'
import { Tabs, TabList, Tab, TabPanel } from './tabs'
import { Tooltip } from './tooltip'
import { StatusBadge, LiveMessage, Progress } from './feedback'
import { ToastProvider, useToast } from './toast'

const themed = (children: React.ReactNode) => <Theme>{children}</Theme>

test('named actions retain keyboard semantics and pending blocks activation without replacing the label', async () => {
 const user = userEvent.setup(), act = vi.fn()
 const { rerender } = render(themed(<Button icon="save" onClick={act}>Save changes</Button>))
 const button = screen.getByRole('button', { name: 'Save changes' })
 await user.tab(); await user.keyboard('{Enter}'); await user.keyboard(' ')
 expect(act).toHaveBeenCalledTimes(2)
 expect(button.querySelector('svg')).toHaveAttribute('aria-hidden', 'true')
 expect(button.querySelector('svg')).toHaveAttribute('stroke-width', '1.75')
 rerender(themed(<Button icon="save" pending onClick={act}>Save changes</Button>))
 expect(button).toHaveAttribute('aria-busy', 'true'); expect(button).toBeDisabled()
 await user.click(button); expect(act).toHaveBeenCalledTimes(2)
 expect(button).toHaveTextContent('Save changes')
})

test('icon-only controls have a supplied accessible name and navigation remains an anchor', () => {
 render(themed(<><IconButton label="Close review" icon="close" /><ActionLink href="/account" icon="user">View account</ActionLink></>))
 expect(screen.getByRole('button', { name: 'Close review' })).toHaveAttribute('type', 'button')
 expect(screen.getByRole('link', { name: 'View account' })).toHaveAttribute('href', '/account')
})

test('field descriptions append and deduplicate caller guidance, and errors reference existing elements only', () => {
 const { rerender } = render(themed(<><p id="external">External guidance</p><Field id="name" label="Name" description="Use the team name" error="Enter a name"><Input aria-describedby="external name-help" aria-invalid={false} /></Field></>))
 const input = screen.getByLabelText('Name')
 expect(input).toHaveAttribute('aria-describedby', 'external name-help name-error')
 expect(input).toHaveAttribute('aria-invalid', 'true')
 for (const id of input.getAttribute('aria-describedby')!.split(' ')) expect(document.getElementById(id)).not.toBeNull()
 rerender(themed(<Field id="name" label="Name"><Input /></Field>))
 expect(screen.getByLabelText('Name')).not.toHaveAttribute('aria-describedby')
 expect(screen.getByLabelText('Name')).not.toHaveAttribute('aria-invalid')
})

test('native control refs and RHF validation own Enter submission and focus the first error', async () => {
 const user = userEvent.setup(), save = vi.fn()
 function Form() {
  const form = useForm({ defaultValues: { name: '' } })
  return <form onSubmit={form.handleSubmit(save)}><Field label="Name" error={form.formState.errors.name?.message}><Input {...form.register('name', { required: 'Enter a name' })} /></Field><Button type="submit" icon="save">Save draft</Button></form>
 }
 render(themed(<Form />))
 const input = screen.getByLabelText('Name')
 await user.click(input); await user.keyboard('{Enter}')
 expect(save).not.toHaveBeenCalled(); await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'true'))
 expect(input).toHaveFocus(); expect(input).toHaveAccessibleDescription('Enter a name')
 await user.type(input, 'Synthetic team{Enter}'); await waitFor(() => expect(save).toHaveBeenCalledOnce())
 expect(save.mock.calls[0][0]).toEqual({ name: 'Synthetic team' })
})

test('select, checkbox and multiline controls retain native names and recovery input', async () => {
 const user = userEvent.setup()
 render(themed(<><Field label="Team"><Select defaultValue=""><option value="">Choose</option><option value="atlas">Atlas</option></Select></Field><label><Checkbox />Include demo</label><Field label="Note"><Textarea defaultValue="Recoverable note" /></Field></>))
 await user.selectOptions(screen.getByLabelText('Team'), 'atlas'); expect(screen.getByLabelText('Team')).toHaveValue('atlas')
 await user.click(screen.getByRole('checkbox')); expect(screen.getByRole('checkbox', { name: 'Include demo' })).toBeChecked()
 expect(screen.getByLabelText('Note')).toHaveValue('Recoverable note')
})

test('explicit invalid focus ignores hidden and disabled controls', () => {
 const { container } = render(<form><div hidden><Input aria-invalid="true" aria-label="Hidden" /></div><fieldset disabled><Input aria-invalid="true" aria-label="Disabled descendant" /></fieldset><fieldset aria-invalid="true"><Input aria-label="Valid child" /></fieldset><div aria-invalid="true"><span>Not a control</span></div><Input aria-invalid="true" aria-label="Current" /></form>)
 focusFirstInvalid(container.querySelector('form')!); expect(screen.getByLabelText('Current')).toHaveFocus()
})

test('an initially open dialog waits for the owning theme portal and keeps an explicit focus target', async () => {
 function Initial() { const input = useRef<HTMLInputElement>(null); return <Theme><Theme accent="#233c78"><Dialog defaultOpen><DialogContent title="Review evidence" description="Only current evidence" initialFocus={input}><Input ref={input} aria-label="Evidence" /></DialogContent></Dialog></Theme></Theme> }
 render(<Initial />)
 const dialog = await screen.findByRole('dialog', { name: 'Review evidence' })
 expect(dialog.closest('[data-theme-portals]')).not.toBeNull()
 expect(dialog.closest('.theme')).toHaveStyle('--primary: #233c78')
 await waitFor(() => expect(screen.getByLabelText('Evidence')).toHaveFocus())
})

test('dialog Escape returns focus to the opener, while the owner retains the draft', async () => {
 const user = userEvent.setup()
 function Editor() { const [value, setValue] = useState(''); return <Dialog><DialogTrigger render={<Button />}>Review draft</DialogTrigger><DialogContent title="Review draft" description="Changes remain local" footer={<DialogClose render={<Button variant="outline" />}>Cancel review</DialogClose>}><Input aria-label="Draft" value={value} onChange={e => setValue(e.target.value)} /></DialogContent></Dialog> }
 render(themed(<Editor />)); const opener = screen.getByRole('button', { name: 'Review draft' })
 await user.click(opener); await user.type(await screen.findByLabelText('Draft'), 'Preserved locally')
 await user.keyboard('{Escape}'); await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument()); expect(opener).toHaveFocus()
 await user.click(opener); expect(await screen.findByLabelText('Draft')).toHaveValue('Preserved locally')
})

test('pending confirmation keeps its task open, prevents duplicate intent and permits Escape again after failure', async () => {
 const user = userEvent.setup(), confirm = vi.fn(), change = vi.fn()
 const { rerender } = render(themed(<ConfirmationDialog open onOpenChange={change} title="Queue TEST?" description="Shared channel" confirmLabel="Queue message" onConfirm={confirm} pending />))
 const dialog = await screen.findByRole('alertdialog', { name: 'Queue TEST?' })
 expect(within(dialog).getByRole('button', { name: 'Queue message' })).toBeDisabled()
 await user.keyboard('{Escape}'); expect(change).not.toHaveBeenCalled(); expect(confirm).not.toHaveBeenCalled()
 rerender(themed(<ConfirmationDialog open onOpenChange={change} title="Queue TEST?" description="Shared channel" confirmLabel="Queue message" onConfirm={confirm} />))
 await user.keyboard('{Escape}'); await waitFor(() => expect(change).toHaveBeenCalledWith(false))
})

test('menus use keyboard navigation, scoped portals and restore focus', async () => {
 const user = userEvent.setup(), choose = vi.fn()
 render(themed(<Menu><MenuTrigger render={<Button />}>Account</MenuTrigger><MenuContent><MenuItem icon="user" onClick={choose}>View account</MenuItem><MenuItem disabled>Unavailable</MenuItem><MenuItem icon="logout">Sign out</MenuItem></MenuContent></Menu>))
 const trigger = screen.getByRole('button', { name: 'Account' }); await user.tab(); await user.keyboard('{ArrowDown}')
 const menu = await screen.findByRole('menu'); expect(menu.closest('[data-theme-portals]')).not.toBeNull()
 await user.keyboard('{Enter}'); expect(choose).toHaveBeenCalledOnce(); await waitFor(() => expect(trigger).toHaveFocus())
})

test('popover is named and scoped, and tooltip supplements an already named control', async () => {
 const user = userEvent.setup()
 render(themed(<><p>Always-visible guidance</p><Popover><PopoverTrigger render={<Button />}>View guidance</PopoverTrigger><PopoverContent title="Guidance">Only explanatory content</PopoverContent></Popover><Tooltip content="Refresh current evidence"><IconButton label="Refresh evidence" icon="refresh" /></Tooltip></>))
 await user.click(screen.getByRole('button', { name: 'View guidance' })); const popup = await screen.findByRole('dialog', { name: 'Guidance' }); expect(popup.closest('[data-theme-portals]')).not.toBeNull()
 await user.keyboard('{Escape}'); await user.tab(); expect(await screen.findByRole('tooltip')).toHaveTextContent('Refresh current evidence')
 expect(screen.getByText('Always-visible guidance')).toBeVisible()
})

test('tabs move focus with arrows and activate only on explicit Enter', async () => {
 const user = userEvent.setup()
 render(themed(<Tabs defaultValue="one"><TabList aria-label="Review sections"><Tab value="one">Summary</Tab><Tab value="two">Evidence</Tab></TabList><TabPanel value="one">Summary content</TabPanel><TabPanel value="two">Evidence content</TabPanel></Tabs>))
 await user.click(screen.getByRole('tab', { name: 'Summary' })); await user.keyboard('{ArrowRight}')
 expect(screen.getByRole('tab', { name: 'Evidence' })).toHaveFocus(); expect(screen.getByRole('tab', { name: 'Summary' })).toHaveAttribute('aria-selected', 'true')
 await user.keyboard('{Enter}'); expect(screen.getByRole('tabpanel')).toHaveTextContent('Evidence content')
})

test('status is visible text without implicit live announcements and unknown progress is not zero', () => {
 render(themed(<><StatusBadge tone="warning">Needs reconciliation</StatusBadge><Progress label="Upload progress" /><Progress label="Known progress" value={3} max={5} /><Progress label="Unavailable maximum" max={Infinity} /><LiveMessage>Draft saved</LiveMessage></>))
 expect(screen.getByText('Needs reconciliation')).not.toHaveAttribute('role')
 expect(screen.getByRole('progressbar', { name: 'Upload progress' })).not.toHaveAttribute('value')
 expect(screen.getByRole('progressbar', { name: 'Known progress' })).toHaveAttribute('value', '3')
 expect(screen.getByRole('progressbar', { name: 'Unavailable maximum' })).toHaveAttribute('max', '100')
 expect(screen.getByRole('status')).toHaveAttribute('aria-atomic', 'true')
})

test('toast feedback remains in its owning theme and clears on scope replacement', async () => {
 const user = userEvent.setup()
 function Notify() { const toast = useToast(); return <Button onClick={() => toast.notify({ title: 'Draft saved', urgent: true })}>Notify</Button> }
 const { rerender } = render(themed(<ToastProvider scopeKey="account-a:event-a"><Notify /></ToastProvider>))
 await user.click(screen.getByRole('button', { name: 'Notify' })); const title = await screen.findByText('Draft saved', { selector: 'h2' })
 expect(title.closest('[data-theme-portals]')).not.toBeNull()
 expect(JSON.stringify(localStorage) + JSON.stringify(sessionStorage)).not.toContain('Draft saved')
 rerender(themed(<ToastProvider scopeKey="account-b:event-b"><Notify /></ToastProvider>)); await waitFor(() => expect(screen.queryAllByText('Draft saved')).toHaveLength(0))
})

 test('controlled confirmation without a Base UI trigger returns focus to its real opener', async () => {
 const user = userEvent.setup()
 function Consumer() { const [open,setOpen]=useState(false), trigger=useRef<HTMLButtonElement>(null);return <><Button ref={trigger} onClick={()=>setOpen(true)}>Queue fixed TEST</Button><ConfirmationDialog open={open} onOpenChange={setOpen} title="Confirm fixed TEST" description="No private content" confirmLabel="Queue message" finalFocus={trigger} onConfirm={()=>{}} /></> }
 render(themed(<Consumer />));const trigger=screen.getByRole('button',{name:'Queue fixed TEST'})
 await user.click(trigger);await waitFor(()=>expect(screen.getByRole('button',{name:'Cancel'})).toHaveFocus())
 await user.keyboard('{Escape}');await waitFor(()=>expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());expect(trigger).toHaveFocus()
 await user.click(trigger);await user.click(await screen.findByRole('button',{name:'Cancel'}));await waitFor(()=>expect(trigger).toHaveFocus())
 })
