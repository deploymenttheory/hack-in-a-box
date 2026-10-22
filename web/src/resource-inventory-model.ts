import type { components } from './api/schema.gen'
export type Inventory = components['schemas']['CredentialPage']
export type Resource = components['schemas']['CredentialMetadata']
export type ResourceCommand = components['schemas']['CredentialCommand']
export type InventoryTeam = components['schemas']['HackTeam']
export const resourceTypes: Record<Resource['resource_type'], string> = { credential: 'Credential', account: 'Account', licence: 'Licence', access_code: 'Access code', other: 'Other' }
export const readinessLabels: Record<Resource['readiness'], string> = { ready: 'Ready', unassigned: 'Unassigned', scheduled: 'Scheduled', expired: 'Expired', revoked: 'Revoked' }
