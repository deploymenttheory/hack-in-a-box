import { Children, isValidElement, useId, useRef, useState, type ReactElement, type ReactNode } from 'react'
import { Button } from './components/ui/button'
import { Label, Select } from './components/ui/fields'
import { Icon, type IconName } from './components/ui/icon'

export function PlanningHeader({ title, context, description, actions }: { title: string; context: string; description?: ReactNode; actions?: ReactNode }) {
  return <header className="fleet-planning-heading"><div><p className="fleet-scope-label">{context}</p><h1>{title}</h1>{description && <div className="fleet-description">{description}</div>}</div>{actions && <div className="fleet-pattern-actions">{actions}</div>}</header>
}
export type PlanningDestination = { id: string; label: string; icon: IconName; group: string }
export function PlanningNavigation({ items, current, choose }: { items: readonly PlanningDestination[]; current: string; choose: (id: string) => void }) {
  const id=useId(),groups=['Event','Plan','Delivery','Workspaces'].filter(group=>items.some(item=>item.group===group))
  return <nav className="fleet-planning-nav" aria-label="Event sections"><div className="fleet-planning-nav-mobile"><Label htmlFor={id}>Event section</Label><Select id={id} value={items.some(item=>item.id===current)?current:items[0]?.id??''} onChange={event=>choose(event.target.value)}>{groups.map(group=><optgroup key={group} label={group}>{items.filter(item=>item.group===group).map(item=><option key={item.id} value={item.id}>{item.label}</option>)}</optgroup>)}</Select></div><div className="fleet-planning-nav-groups">{groups.map(group => <section key={group}><h2>{group}</h2><ul>{items.filter(item => item.group === group).map(item => <li key={item.id}><Button variant="ghost" type="button" aria-current={current === item.id ? 'page' : undefined} onClick={() => choose(item.id)}><Icon name={item.icon} />{item.label}</Button></li>)}</ul></section>)}</div></nav>
}
export function PlanningStep({ children }: { title: string; children: ReactNode }) { return <>{children}</> }
// Inactive editors remain mounted: moving between sections never discards an
// unsent value or creates a new command. The domain owner still owns submission.
export function PlanningEditorSections({ children, label = 'Configuration sections', enabled = true }: { children: ReactNode; label?: string; enabled?: boolean }) {
  const id = useId(), [active, setActive] = useState(0), root = useRef<HTMLDivElement>(null)
  const steps = Children.toArray(children).filter((child): child is ReactElement<{title: string; children: ReactNode}> => isValidElement<{title: string; children: ReactNode}>(child) && child.type === PlanningStep)
  if (!enabled) return <>{children}</>
  return <div className="fleet-editor-sections" ref={root} onInvalidCapture={event => { const panel = (event.target as HTMLElement).closest<HTMLElement>('[data-planning-panel]'); if (panel) { setActive(Number(panel.dataset.planningPanel)); const field = event.target as HTMLElement; requestAnimationFrame(() => field.focus()) } }}>
    <nav aria-label={label}>{steps.map((step, index) => isValidElement<{title: string}>(step) && <Button key={step.key ?? index} type="button" variant="ghost" aria-current={active === index ? 'step' : undefined} aria-controls={`${id}-${index}`} onClick={() => setActive(index)}>{step.props.title}</Button>)}</nav>
    <div>{steps.map((step, index) => <section key={step.key ?? index} id={`${id}-${index}`} data-planning-panel={index} hidden={active !== index}>{step}</section>)}</div>
  </div>
}
// RHF validation is not a native invalid event. Reveal its existing field before
// focusing it; this changes presentation only, never data or validation rules.
export function revealPlanningField(form: HTMLFormElement | null, name: string) {
  const field = form?.querySelector<HTMLElement>(`[name="${CSS.escape(name)}"], [id="${CSS.escape(name)}"]`)
  const panel = field?.closest<HTMLElement>('[data-planning-panel]')
  if (panel) { const root = panel.closest('.fleet-editor-sections'); const control = root?.querySelector<HTMLButtonElement>(`[aria-controls="${CSS.escape(panel.id)}"]`); control?.click() }
  requestAnimationFrame(() => field?.focus())
}
const destinations: Record<string, {group: string; icon: IconName}> = {
 repositories:{group:'Workspaces',icon:'repository'},overview:{group:'Event',icon:'layout'},today:{group:'Event',icon:'check'},'review coverage':{group:'Event',icon:'shield'},
 setup:{group:'Plan',icon:'calendar'},rules:{group:'Plan',icon:'book'},schedule:{group:'Plan',icon:'calendar'},branding:{group:'Plan',icon:'palette'},objectives:{group:'Plan',icon:'list'},staff:{group:'Plan',icon:'user'},preparation:{group:'Plan',icon:'check'},inventory:{group:'Plan',icon:'columns'},
 attendance:{group:'Delivery',icon:'team'},operations:{group:'Delivery',icon:'check'},logistics:{group:'Delivery',icon:'shield'},
}
export function eventPlanningDestinations(names: readonly string[]): PlanningDestination[] { return names.map(id=>({id,label:id==='mc console'?'MC console':id[0].toUpperCase()+id.slice(1),...(destinations[id]??{group:'Workspaces',icon:'book' as const})})) }

export function firstPlanningError(errors: unknown, path = ''): string {
  if (!errors || typeof errors !== 'object') return ''
  if ('message' in errors || 'type' in errors) return path
  for (const [key,value] of Object.entries(errors)) { const name=firstPlanningError(value,path?path+'.'+key:key); if(name)return name }
  return ''
}

export function usePlanningActionFocus() {
 const actions=useRef<Record<string,HTMLButtonElement|null>>({})
 return { bind:(id:string)=>(element:HTMLButtonElement|null)=>{actions.current[id]=element}, restore:(id:string|undefined)=>requestAnimationFrame(()=>{if(id)actions.current[id]?.focus()}) }
}
