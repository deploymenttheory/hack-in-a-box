import { StrictMode, useEffect, useRef, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { Button } from '../components/ui/button'
import { Icon, type IconName } from '../components/ui/icon'
import { PreviewFrame } from './preview-frame'
import { palettes, type Mode, type Palette } from './preferences'
import { UsageReference } from './reference'
import { Snippet } from './snippet'
import { Theme } from '../components/theme'
import { Field, Select } from '../components/ui/fields'
import { brandStyle } from '../design-system/palette'
import { contributionSteps, principles, sections, tokenGroups, type Section } from './content'
import qualities from './qualities.json'
import './production.css'
import './styleguide.css'

function sectionFromHash(): Section {
  const value = location.hash.slice(1)
  return sections.find(section => section.id === value)?.id ?? (value.startsWith('quality-') ? 'qualities' : 'overview')
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
function App() {
  const [section, setSection] = useState<Section>(sectionFromHash)
  const [mode, setMode] = useState<Mode>('system')
  const [palette, setPalette] = useState<Palette>('platform')
  const [density, setDensity] = useState<'comfortable' | 'compact'>('comfortable')
  const [width, setWidth] = useState('full')
  const [portal, setPortal] = useState<'admin' | 'participant'>('admin')
  const [resetKey, setResetKey] = useState(0)
  useEffect(() => { const change = () => setSection(sectionFromHash()); addEventListener('hashchange', change); return () => removeEventListener('hashchange', change) }, [])
  useEffect(() => { if (/^#quality-q\d{2,3}$/.test(location.hash)) document.getElementById(location.hash.slice(1))?.scrollIntoView() }, [section])
  const title = sections.find(item => item.id === section)!.label
  const revision = import.meta.env.VITE_STYLEGUIDE_REVISION as string | undefined
  const navIcons: Record<Section, IconName> = { overview: 'book', tokens: 'palette', typography: 'type', components: 'layout', patterns: 'list', qualities: 'check', contributing: 'code' }
  const version = revision && /^[a-f0-9]{7,40}$/i.test(revision) ? revision.slice(0, 12) : 'Local build'
  return <Theme mode={mode} density={density}><a className="skip-link" href="#guide-main">Skip to guide content</a><div className="guide-shell">
    <header className="guide-header"><a href="#overview" className="guide-brand"><Icon name="book" />Hack-in-a-Box <span>Style guide</span></a><span className="guide-version">{version}</span></header>
    <div className="guide-controls" aria-label="Guide preview controls"><Field id="guide-theme" label="Theme"><Select value={mode} onChange={event => setMode(event.target.value as Mode)}><option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option></Select></Field><Field id="guide-palette" label="Example palette"><Select value={palette} onChange={event => setPalette(event.target.value as Palette)}><option value="platform">Platform</option><option value="light">Custom light</option><option value="dark">Custom dark</option><option value="threshold">Threshold contrast</option></Select></Field><Field id="guide-density" label="Density"><Select value={density} onChange={event => setDensity(event.target.value as typeof density)}><option value="comfortable">Comfortable</option><option value="compact">Compact</option></Select></Field><Field id="guide-width" label="Example viewport" description="Applies to interactive component and task pattern examples."><Select disabled={section !== 'components' && section !== 'patterns'} value={width} onChange={event => setWidth(event.target.value)}><option value="full">Available width</option><option value="736">736px</option><option value="360">360px</option></Select></Field><Button icon="refresh" variant="outline" onClick={() => setResetKey(value => value + 1)}>Reset examples</Button></div>
    <p className="guide-synthetic">Interactive documentation with synthetic examples. This guide does not connect to your event, account or providers.</p>
    <div className="guide-layout"><nav className="guide-nav" aria-label="Guide sections">{sections.map(item => <a href={`#${item.id}`} key={item.id} aria-current={section === item.id ? 'page' : undefined}><Icon name={navIcons[item.id]} />{item.label}</a>)}</nav>
    <main id="guide-main" className="guide-main" tabIndex={-1}><div className="guide-preview"><Theme mode={mode} density={density} style={palette === 'platform' ? undefined : brandStyle(palettes[palette])}><div className="guide-content"><h1>{title}</h1><span className="sr-only" role="status">{title}</span>
      {section === 'overview' && <><p>The accepted Fleet-inspired system gives people a clear route from current work to its next action. Use the same production tokens, controls and task patterns in both independent portals.</p><ol className="guide-principles">{principles.map(([heading, text]) => <li key={heading}><h2>{heading}</h2><p>{text}</p></li>)}</ol></>}
      {section === 'tokens' && <><p>These values are read from the rendered production theme. Change the controls above to inspect system modes, event palette isolation and density.</p><TokenTable refresh={`${mode}:${palette}:${density}`} /><Snippet>{'<Theme mode="system" density="comfortable">\n  <YourWorkspace />\n</Theme>'}</Snippet></>}
      {section === 'typography' && <><p>Self-hosted Inter uses a small rem-based scale and regular/semibold emphasis. Text remains readable with browser zoom and a system-font fallback.</p><section className="guide-section"><p className="guide-type-page">Page title · 24px</p><p className="guide-type-section">Section heading · 16px</p><p>Body and actions · 14px</p><p className="guide-type-caption">Supporting metadata · 12px</p></section><h2>Spacing</h2><ul className="guide-spacing">{[1, 2, 3, 4, 6, 8].map(step => <li key={step}><code>--space-{step}</code><span aria-hidden="true" style={{ inlineSize: `var(--space-${step})` }} /><span>{step * 4}px</span></li>)}</ul></>}
      {section === 'components' && <><PreviewFrame options={{ kind: 'guide-preview-options', mode, palette, density, resetKey, portal }} width={width} /><UsageReference key={resetKey} kind="primitives" /></>}
      {section === 'patterns' && <><Field id="guide-portal" label="Synthetic portal"><Select value={portal} onChange={event => setPortal(event.target.value as typeof portal)}><option value="admin">Administrator</option><option value="participant">Participant</option></Select></Field><PreviewFrame story="patterns" options={{ kind: 'guide-preview-options', mode, palette, density, resetKey, portal }} width={width} /><p>Keep event and entity scope visible through each decision. A workspace supports general work, a review desk supports evidence and an action queue helps people resolve the next exception.</p><UsageReference key={resetKey} kind="patterns" /><ol className="guide-principles"><li><h2>Collection → detail</h2><p>Align comparable records, name the detail link and preserve useful filters when returning. Counts and pagination must reflect the actual data contract.</p></li><li><h2>Review → confirmation</h2><p>Show the exact affected entity and action before committing. Keep cancellation available according to the operation’s safe dismissal rules.</p></li><li><h2>Receipt → next action</h2><p>Use the owning response to show completion. A queued invitation is not provider acceptance; an unknown effect offers status reconciliation without replay.</p></li></ol></>}
      {section === 'qualities' && <QualityGuidelines />}
      {section === 'contributing' && <><p>Future additions and changes maintain one production system rather than creating a separate set of examples.</p><ol className="guide-principles">{contributionSteps.map((step, index) => <li key={step}><h2>Step {index + 1}</h2><p>{step}</p></li>)}</ol><p>All UI changes retain the project’s current identity, event/resource permissions, lifecycle and receipt contracts. The complete browser/accessibility and event rehearsal gates remain separately required.</p></>}
    </div></Theme></div></main></div>
    <footer className="guide-footer">Production components and guidelines · <a href="/assets/licenses/Inter-OFL-1.1.txt">Inter font license</a> · <a href="https://github.com/deploymenttheory/hack-in-a-box/issues/227" target="_blank" rel="noopener noreferrer">Accepted design baseline</a></footer>
  </div></Theme>
}
createRoot(document.getElementById('root')!).render(<StrictMode><App /></StrictMode>)
