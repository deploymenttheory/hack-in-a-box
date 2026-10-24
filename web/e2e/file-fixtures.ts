import { createHash } from 'node:crypto'
import type { components } from '../src/api/schema.gen'
import { firstID, person } from './fixtures'

// Match the generated native File projection, including actual byte length.
export function uploadedFile(id: string, body: { filename: string; media_type: string; content: string }): components['schemas']['File'] {
  const bytes = Buffer.from(body.content, 'base64')
  return { id, event_id: firstID, owner_id: person.id, filename: body.filename, media_type: body.media_type, size_bytes: bytes.length, checksum: createHash('sha256').update(bytes).digest('hex'), created_at: '2027-06-14T09:00:00Z' }
}
