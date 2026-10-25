import { webcrypto } from 'node:crypto'
import { afterEach, expect, test, vi } from 'vitest'
import vectors from './repository-access-fingerprint-vectors.json'
import { repositoryIntentFingerprint, repositoryReason, type RepositoryIntentIdentity } from './repository-fingerprint'
afterEach(() => vi.unstubAllGlobals())
test('native and browser command fingerprints share UTF-8 length-framed golden identity for Unicode and literal markup', async () => {
  vi.stubGlobal('crypto', webcrypto)
  for (const vector of vectors) expect(await repositoryIntentFingerprint(vector.input as RepositoryIntentIdentity)).toBe(vector.expected)
  const original = vectors[1].input as RepositoryIntentIdentity
  for (const changed of [{ ...original, reason: original.reason + ' changed' }, { ...original, repositoryID: original.repositoryID + 1 }, { ...original, version: original.version! + 1 }, { ...original, actorID: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' }]) expect(await repositoryIntentFingerprint(changed)).not.toBe(vectors[1].expected)
  expect(repositoryReason('\u0085 synthetic \u0085')).toBe('synthetic')
  await expect(repositoryIntentFingerprint({ ...original, repositoryID: Number.MAX_SAFE_INTEGER + 1 })).rejects.toThrow('not representable safely')
})
