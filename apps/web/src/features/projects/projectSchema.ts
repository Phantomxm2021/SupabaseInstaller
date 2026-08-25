import { z } from 'zod'
import type { Services } from '../../api/types'

export const lightweightServices: Services = {
  database: true,
  gateway: true,
  auth: true,
  rest: true,
  studio: true,
  postgresMeta: true,
  realtime: false,
  storage: false,
  imgproxy: false,
  functions: false,
  supavisor: false,
  logs: false,
  vector: false,
  directDb: false,
}

export const projectSchema = z.object({
  name: z.string().trim().min(1).max(80),
  slug: z.string().regex(/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/, 'Use lowercase letters, numbers, and hyphens'),
  domain: z.string().min(1).refine((value) => value === 'localhost' || /^[a-z0-9.-]+(?::\d+)?$/.test(value), 'Enter a hostname without http://'),
  siteUrl: z.url().refine((value) => value.startsWith('https://') || value.startsWith('http://'), 'Enter an http or https URL'),
})

export type ProjectForm = z.infer<typeof projectSchema>

export function slugify(value: string) {
  return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 63)
}
