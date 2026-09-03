export type { RedactedProjectConfiguration } from '../../../api/types'

/** Explicit camelCase wire names for hardened configuration fields. */
export type HardenedConfigurationWire = {
  authSiteUrl: string
  internalDbPoolSize: number
}
