import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'
import { useState } from 'react'
import { Theme } from './components/theme'
import { Input, Label } from './components/ui/fields'
import { PlanningEditorSections, PlanningNavigation, PlanningStep, eventPlanningDestinations, firstPlanningError } from './planning-layout'

test('focused planning sections retain mounted private edits and native keyboard navigation',async()=>{
 const user=userEvent.setup()
 function Editor(){const[value,setValue]=useState('');return <Theme><PlanningEditorSections><PlanningStep title="Identity"><Label htmlFor="private-note">Private note</Label><Input id="private-note" value={value} onChange={e=>setValue(e.target.value)}/></PlanningStep><PlanningStep title="Review"><h2>Review the current command</h2><p>{value}</p></PlanningStep></PlanningEditorSections></Theme>}
 render(<Editor/>);await user.type(screen.getByLabelText('Private note'),'Retained in memory')
 const review=screen.getByRole('button',{name:'Review'});review.focus();await user.keyboard('{Enter}')
 expect(review).toHaveAttribute('aria-current','step');expect(screen.getByRole('heading',{name:'Review the current command'})).toBeVisible()
 expect(document.getElementById('private-note')).toHaveValue('Retained in memory')
 await user.click(screen.getByRole('button',{name:'Identity'}));expect(screen.getByLabelText('Private note')).toHaveValue('Retained in memory')
})
test('native invalid field in an inactive planning section becomes visible without changing its value',async()=>{
 const user=userEvent.setup()
 render(<Theme><PlanningEditorSections><PlanningStep title="First"><p>First section</p></PlanningStep><PlanningStep title="Dates"><Label htmlFor="required-date">Due date</Label><Input id="required-date" required defaultValue=""/></PlanningStep></PlanningEditorSections></Theme>)
 const field=document.getElementById('required-date')!
 fireEvent.invalid(field)
 expect(screen.getByLabelText('Due date')).toBeVisible();expect(screen.getByLabelText('Due date')).toHaveValue('')
 await user.click(screen.getByRole('button',{name:'First'}));expect(screen.queryByRole('textbox',{name:'Due date'})).not.toBeInTheDocument()
})
test('event navigation offers only the supplied current capability registry and does not infer a grant',async()=>{
 const choose=vi.fn(),user=userEvent.setup()
 render(<Theme><PlanningNavigation items={eventPlanningDestinations(['overview','resources'])} current="overview" choose={choose}/></Theme>)
 const nav=screen.getByRole('navigation',{name:'Event sections'})
 expect(within(nav).queryByRole('button',{name:'Staff'})).not.toBeInTheDocument()
 await user.click(within(nav).getByRole('button',{name:'Resources'}));expect(choose).toHaveBeenCalledExactlyOnceWith('resources')
 expect(within(nav).getByRole('button',{name:'Overview'})).toHaveAttribute('aria-current','page')
})
test('nested native validation paths retain their exact domain field name',()=>{
 expect(firstPlanningError({definition:{checklist:[{title:{message:'Required',type:'too_small'}}]}})).toBe('definition.checklist.0.title')
 expect(firstPlanningError({})).toBe('')
})
