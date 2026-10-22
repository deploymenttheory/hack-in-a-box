import { useState } from 'react'
import { SafeMarkdown } from './components/safe-markdown'
import { blockerText, type CampaignDetail, type MessageTemplate } from './campaign-model'

// The server's HTML format contains escaped literal text plus its own <br> tags.
// Decode only that bounded escaping and render it as React text; never parse a
// message as DOM/HTML or load its images/links in the organiser's session.
function literalHTML(value: string) {
  const entities: Record<string, string> = { '&amp;': '&', '&lt;': '<', '&gt;': '>', '&#34;': '"', '&#39;': "'" }
  return value.replaceAll('<br>\n', '\n').replace(/&(?:amp|lt|gt|#34|#39);/g, value => entities[value])
}
export function CampaignPreview({ detail, template }: { detail?: CampaignDetail; template?: MessageTemplate }) {
  const [selected, setSelected] = useState('')
  const preview = detail?.preview, recipients = preview?.recipients ?? [], shown = recipients.slice(0, 200), person = shown.find(p => p.person_id === selected) ?? shown[0]
  return <section className="panel" aria-label="Personalised campaign preview"><h3>Personalised preview</h3>{!preview ? <p>Save the campaign, then generate a retained preview of the current eligible audience.</p> : <><p><strong>{recipients.length} eligible recipients in this preview</strong> · Message version {preview.template_version} · Audience version {preview.audience_version}</p><p>This is a retained review snapshot. Execution resolves current eligible membership again; approval does not publish content or prove delivery.</p>{preview.blockers.map(code => <p className="notice" key={code}>{blockerText(code)}</p>)}{!!shown.length && <><label htmlFor="preview-person">Preview recipient</label><select id="preview-person" value={person?.person_id ?? ''} onChange={e => setSelected(e.target.value)}>{shown.map(p => <option key={p.person_id} value={p.person_id}>{p.name}</option>)}</select>{recipients.length > shown.length && <p>Showing the first {shown.length} personalised samples; the retained audience count includes everyone.</p>}{person && <article className="campaign-message-preview" aria-label="Rendered message"><h4>{person.subject}</h4>{template?.version !== preview.template_version && <p className="notice">This retained preview uses an older message version. Its source is shown as text; generate a new preview before approval.</p>}{template?.version === preview.template_version && template.format === 'markdown' ? <SafeMarkdown>{person.body}</SafeMarkdown> : <p className="campaign-message-text">{template?.version === preview.template_version && template.format === 'html' ? literalHTML(person.body) : person.body}</p>}{person.unresolved.length > 0 && <p role="alert">Unresolved for this recipient: {person.unresolved.join(', ')}</p>}</article>}</>}</>}</section>
}
