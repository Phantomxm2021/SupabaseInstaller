export type HealthStatus = 'HEALTHY' | 'DEGRADED' | 'STARTING' | 'STOPPED' | 'UNHEALTHY' | 'UNKNOWN'
export type ProjectStatus = 'DRAFT' | 'INSTALLING' | 'RUNNING' | 'STOPPED' | 'DEGRADED' | 'FAILED' | 'DELETING'

export interface Services {
  database: boolean
  gateway: boolean
  auth: boolean
  rest: boolean
  studio: boolean
  postgresMeta: boolean
  realtime: boolean
  storage: boolean
  imgproxy: boolean
  functions: boolean
  supavisor: boolean
  logs: boolean
  vector: boolean
  directDb: boolean
}

export interface Project {
  id: string
  name: string
  slug: string
  domain: string
  siteUrl: string
  status: ProjectStatus
  health: HealthStatus
  supabaseVersion: string
  preset: 'LIGHTWEIGHT' | 'STANDARD' | 'FULL' | 'CUSTOM'
  services: Services
  createdAt: string
  updatedAt: string
}

export interface Operation {
  id: string
  projectId: string
  type: string
  status: 'QUEUED' | 'RUNNING' | 'SUCCEEDED' | 'FAILED' | 'ROLLING_BACK' | 'ROLLED_BACK' | 'CANCELLED'
  currentStep?: string
  progress: number
  errorCode?: string
  errorMessage?: string
}
