import { expect, test } from 'vitest'
import { gitHubSetupErrors, type GitHubSetupValues } from './github-setup-fields'
const values: GitHubSetupValues = { appID: '42', slug: 'synthetic-app', orgID: '99', organization: 'synthetic-org', template: 'starter', privateKey: 'SYNTHETIC-NOT-A-REAL-KEY' }
test('setup format checks reject client IDs, URLs, noninteger numeric syntax and oversized keys without echoing secrets', () => {
  expect(gitHubSetupErrors(values)).toEqual({})
  for (const appID of ['Iv23-client-id', '1e3', '1.5', '-1', '0', '9007199254740992']) expect(gitHubSetupErrors({ ...values, appID })).toHaveProperty('appID')
  expect(gitHubSetupErrors({ ...values, template: 'org/repo' })).toHaveProperty('template')
  expect(gitHubSetupErrors({ ...values, privateKey: 'é'.repeat(16385) })).toHaveProperty('privateKey')
  expect(JSON.stringify(gitHubSetupErrors({ ...values, privateKey: '' }))).not.toContain(values.privateKey)
})
