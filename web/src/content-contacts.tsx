import type { ContentDocument } from './content-model'
import { safeContactURL } from './content-model'

export function ContentContacts({ document }: { document: ContentDocument }) {
  if (!document.contacts?.length) return null
  return <section className="fleet-resource-section" aria-label="Published contact routes"><h4>{document.purpose === 'conduct' ? 'Conduct reporting contact' : 'Support contact'}</h4><ul>{document.contacts.map((contact, index) => <li key={index}>{safeContactURL(contact.url) ? <a href={contact.url} target="_blank" rel="noopener noreferrer">{contact.label}</a> : <span>{contact.label || 'Contact'} · Route needs configuration</span>}</li>)}</ul>{document.purpose === 'conduct' && <p>This app does not collect reports. Use the published route; external services have their own privacy arrangements.</p>}</section>
}
