import { StrictMode, useEffect, useRef, useState, type CSSProperties } from 'react'
import { createRoot } from 'react-dom/client'
import { Button } from '../components/ui/button'
import { Theme } from '../components/theme'
import { brandStyle, defaultBranding, type Branding } from '../design-system/palette'
import { contributionSteps, principles, sections, tokenGroups, type Section } from './content'
import qualities from './qualities.json'
import '../styles.css'
import './styleguide.css'

type Mode = 'system' | 'light' | 'dark'
type Palette = 'platform' | 'light' | 'dark' | 'threshold'
const palettes: Record<Exclude<Palette, 'platform'>, Branding> = {
  light: { ...defaultBranding, primary: '#233c78', foreground: '#283b51', background: '#fff8e9', accent: '#ffedbb' },
  dark: { ...defaultBranding, primary: '#b1d2ff', foreground: '#eaf1ff', background: '#172238', accent: '#160f30' },
  threshold: { ...defaultBranding, primary: '#767676', foreground: '#767676', background: '#ffffff' },
}
function sectionFromHash(): Section {
  const value = location.hash.slice(1)
  return sections.find(section => section.id === value)?.id ?? (value.startsWith('quality-') ? 'qualities' : 'overview')
}
function Snippet({ children }: { children: string }) {
  const [message, setMessage] = useState('')
  async function copy() {
    try { await navigator.clipboard.writeText(children); setMessage('Code copied.') }
    catch { setMessage('Select and copy the code below.') }
  }
  return <div className="guide-snippet"><div className="guide-snippet-toolbar"><span>Usage</span><Button variant="ghost" size="sm" onClick={() => void copy()}>Copy code</Button><span role="status">{message}</span></div><pre><code>{children}</code></pre></div>
}
function TokenTable({ refresh }: { refresh: string }) {
  const ref = useRef<HTMLDivElement>(null)
  const [values, setValues] = useState<Record<string, string>>({})
  useEffect(() => {
    const read = () => {
      if (!ref.current) return
      const style = getComputedStyle(ref.current)
      setValues(Object.fromEntries(tokenGroups.flatMap(group => group.variables.map(variable => [variable, style.getPropertyValue(variable).trim()]))))
    }
    read()
    const media = matchMedia('(prefers-color-scheme: dark)')
    media.addEventListener('change', read)
    return () => media.removeEventListener('change', read)
  }, [refresh])
  return <div ref={ref}>{tokenGroups.map(group => <section className="guide-section" key={group.name}><h2>{group.name}</h2><div className="guide-table-scroll"><table><caption className="sr-only">{group.name}: current computed production values</caption><thead><tr><th scope="col">Role</th><th scope="col">Current value</th></tr></thead><tbody>{group.variables.map(variable => <tr key={variable}><th scope="row"><code>{variable}</code></th><td>{group.name === 'Colour roles' && <span aria-hidden="true" className="guide-swatch" style={{ backgroundColor: `var(${variable})` }} />}<code>{values[variable] || '—'}</code></td></tr>)}</tbody></table></div></section>)}</div>
}
function QualityGuidelines() {
  const [search, setSearch] = useState('')
  const filtered = qualities.filter(quality => `${quality.id} ${quality.principle} ${quality.guidance}`.toLowerCase().includes(search.trim().toLowerCase()))
  return <><label className="guide-search">Find a guideline<input value={search} onChange={event => setSearch(event.target.value)} type="search" /></label><p role="status">{filtered.length} of {qualities.length} guidelines</p>{!filtered.length && <div className="guide-empty"><h2>No matching guidelines</h2><Button variant="outline" onClick={() => setSearch('')}>Clear search</Button></div>}<ol className="guide-quality-list">{filtered.map(quality => <li id={`quality-${quality.id.toLowerCase()}`} key={quality.id}><h2><a href={`#quality-${quality.id.toLowerCase()}`}>{quality.id}</a> {quality.principle}</h2><p>{quality.guidance}</p><small>Foundation owner: {quality.owner}. Production usage and examples are maintained in this guide.</small></li>)}</ol></>
}
function FoundationComponents() {
  return <section className="guide-section"><h2>Actions</h2><p>Use one specific primary action. Keep cancellation and secondary work quiet. Give icon-only controls a name, and never use a primary action icon without visible text.</p><div className="guide-example" aria-label="Production button variants"><Button>Save draft</Button><Button variant="outline">Cancel</Button><Button variant="secondary">View status</Button><Button variant="ghost">More options</Button><Button variant="destructive">Remove access</Button><Button disabled>Unavailable action</Button></div><Snippet>{'<Button>Save draft</Button>\n<Button variant="outline">Cancel</Button>'}</Snippet><p>Use native forms for Enter submission. The owning operation handles validation, request identity and uncertain effects; a button does not decide that a command succeeded.</p></section>
}
function App() {
  const [section, setSection] = useState<Section>(sectionFromHash)
  const [mode, setMode] = useState<Mode>('system')
  const [palette, setPalette] = useState<Palette>('platform')
  const [density, setDensity] = useState<'comfortable' | 'compact'>('comfortable')
  const [width, setWidth] = useState('full')
  useEffect(() => { const change = () => setSection(sectionFromHash()); addEventListener('hashchange', change); return () => removeEventListener('hashchange', change) }, [])
  const title = sections.find(item => item.id === section)!.label
  const revision = import.meta.env.VITE_STYLEGUIDE_REVISION as string | undefined
  const version = revision && /^[a-f0-9]{7,40}$/i.test(revision) ? revision.slice(0, 12) : 'Local build'
  return <Theme mode={mode} density={density}><a className="skip-link" href="#guide-main">Skip to guide content</a><div className="guide-shell">
    <header className="guide-header"><a href="#overview" className="guide-brand">Hack-in-a-Box <span>Style guide</span></a><span className="guide-version">{version}</span></header>
    <div className="guide-controls" aria-label="Guide preview controls"><label>Theme<select value={mode} onChange={event => setMode(event.target.value as Mode)}><option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option></select></label><label>Example palette<select value={palette} onChange={event => setPalette(event.target.value as Palette)}><option value="platform">Platform</option><option value="light">Custom light</option><option value="dark">Custom dark</option><option value="threshold">Threshold contrast</option></select></label><label>Density<select value={density} onChange={event => setDensity(event.target.value as typeof density)}><option value="comfortable">Comfortable</option><option value="compact">Compact</option></select></label><label>Content width<select value={width} onChange={event => setWidth(event.target.value)}><option value="full">Available width</option><option value="736">736px</option><option value="360">360px</option></select></label></div>
    <p className="guide-synthetic">Interactive documentation with synthetic examples. This guide does not connect to your event, account or providers.</p>
    <div className="guide-layout"><nav className="guide-nav" aria-label="Guide sections">{sections.map(item => <a href={`#${item.id}`} key={item.id} aria-current={section === item.id ? 'page' : undefined}>{item.label}</a>)}</nav>
    <main id="guide-main" className="guide-main" tabIndex={-1}><div className="guide-preview" style={{ maxInlineSize: width === 'full' ? undefined : `${width}px` } as CSSProperties}><Theme mode={mode} density={density} style={palette === 'platform' ? undefined : brandStyle(palettes[palette])}><div className="guide-content"><h1>{title}</h1><span className="sr-only" role="status">{title}</span>
      {section === 'overview' && <><p>The accepted Fleet-inspired system gives people a clear route from current work to its next action. Use the same production tokens, controls and task patterns in both independent portals.</p><ol className="guide-principles">{principles.map(([heading, text]) => <li key={heading}><h2>{heading}</h2><p>{text}</p></li>)}</ol></>}
      {section === 'tokens' && <><p>These values are read from the rendered production theme. Change the controls above to inspect system modes, event palette isolation and density.</p><TokenTable refresh={`${mode}:${palette}:${density}`} /><Snippet>{'<Theme mode="system" density="comfortable">\n  <YourWorkspace />\n</Theme>'}</Snippet></>}
      {section === 'typography' && <><p>Self-hosted Inter uses a small rem-based scale and regular/semibold emphasis. Text remains readable with browser zoom and a system-font fallback.</p><section className="guide-section"><p className="guide-type-page">Page title · 24px</p><p className="guide-type-section">Section heading · 16px</p><p>Body and actions · 14px</p><p className="guide-type-caption">Supporting metadata · 12px</p></section><h2>Spacing</h2><ul className="guide-spacing">{[1, 2, 3, 4, 6, 8].map(step => <li key={step}><code>--space-{step}</code><span aria-hidden="true" style={{ inlineSize: `var(--space-${step})` }} /><span>{step * 4}px</span></li>)}</ul></>}
      {section === 'components' && <FoundationComponents />}
      {section === 'patterns' && <><p>Keep event and entity scope visible through each decision. A workspace supports general work, a review desk supports evidence and an action queue helps people resolve the next exception.</p><ol className="guide-principles"><li><h2>Collection → detail</h2><p>Align comparable records, name the detail link and preserve useful filters when returning. Counts and pagination must reflect the actual data contract.</p></li><li><h2>Review → confirmation</h2><p>Show the exact affected entity and action before committing. Keep cancellation available according to the operation’s safe dismissal rules.</p></li><li><h2>Receipt → next action</h2><p>Use the owning response to show completion. A queued invitation is not provider acceptance; an unknown effect offers status reconciliation without replay.</p></li></ol></>}
      {section === 'qualities' && <QualityGuidelines />}
      {section === 'contributing' && <><p>Future additions and changes maintain one production system rather than creating a separate set of examples.</p><ol className="guide-principles">{contributionSteps.map((step, index) => <li key={step}><h2>Step {index + 1}</h2><p>{step}</p></li>)}</ol><p>All UI changes retain the project’s current identity, event/resource permissions, lifecycle and receipt contracts. The complete browser/accessibility and event rehearsal gates remain separately required.</p></>}
    </div></Theme></div></main></div>
    <footer className="guide-footer">Production components and guidelines · <a href="/assets/licenses/Inter-OFL-1.1.txt">Inter font license</a> · <a href="https://github.com/deploymenttheory/hack-in-a-box/issues/227" target="_blank" rel="noopener noreferrer">Accepted design baseline</a></footer>
  </div></Theme>
}
createRoot(document.getElementById('root')!).render(<StrictMode><App /></StrictMode>)
