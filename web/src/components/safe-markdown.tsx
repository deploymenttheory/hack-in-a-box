import Markdown from 'react-markdown'

// Only deliberate, ordinary links leave the page. Markdown never loads remote
// images or runs raw HTML, and links cannot embed credentials or active schemes.
export function safeContentURL(raw: string): string | undefined {
  if (raw.startsWith('#') && !/[\s<>]/.test(raw)) return raw
  try {
    const url = new URL(raw)
    if (!['https:', 'http:', 'mailto:'].includes(url.protocol) || url.username || url.password) return undefined
    return url.href
  } catch { return undefined }
}
export function SafeMarkdown({ children }: { children: string }) {
  return <div className="content-markdown"><Markdown skipHtml urlTransform={safeContentURL} components={{
    a: ({ href, children }) => href ? <a href={href} target={href.startsWith('#') ? undefined : '_blank'} rel="noopener noreferrer">{children}</a> : <span>{children}</span>,
    img: ({ alt }) => <span className="muted">Image: {alt || 'no description provided'}</span>,
    h1: ({ children }) => <h3>{children}</h3>,
    h2: ({ children }) => <h4>{children}</h4>,
  }}>{children}</Markdown></div>
}
