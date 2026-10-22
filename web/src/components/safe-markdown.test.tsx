import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SafeMarkdown } from './safe-markdown'

describe('native content rendering', () => {
  it('renders headings, lists, code and ordinary links without active HTML or image requests', () => {
    const { container } = render(<SafeMarkdown>{'# Brief\n\n- Build\n- Share\n\n`example`\n\n[Guide](https://example.test/guide)\n\n[Unsafe](javascript:alert%281%29)\n\n![Remote](https://example.test/tracker.png)\n\n<script>alert(1)</script>\n\n<iframe src="https://example.test/frame"></iframe>\n\n[Credentials](https://user:secret@example.test/)'}</SafeMarkdown>)
    expect(screen.getByRole('heading', { name: 'Brief' })).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByRole('link', { name: 'Guide' })).toHaveAttribute('rel', 'noopener noreferrer')
    expect(screen.queryByRole('link', { name: 'Unsafe' })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Credentials' })).not.toBeInTheDocument()
    expect(container.querySelector('script, iframe, img')).toBeNull()
    expect(screen.getByText('Image: Remote')).toBeInTheDocument()
  })
})
