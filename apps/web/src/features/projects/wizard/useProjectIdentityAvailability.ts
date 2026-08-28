import { useEffect, useMemo, useState } from 'react'
import { z } from 'zod'
import type { Project } from '../../../api/types'
import { projectSlugSchema } from '../projectSchema'
import type { Availability } from './types'

const debounceMs = 400
const projectNameSchema = z.string().trim().min(1).max(80)
const idle: Availability = { status: 'idle' }

export interface ProjectIdentityAvailability {
  name: Availability
  slug: Availability
}

function normalize(value: string) {
  return value.trim().toLowerCase()
}

function isValidName(name: string) {
  return projectNameSchema.safeParse(name).success
}

function isValidSlug(slug: string) {
  return projectSlugSchema.safeParse(slug).success
}

function checking(valid: boolean): Availability {
  return valid ? { status: 'checking' } : idle
}

function unavailable(valid: boolean, field: 'name' | 'slug'): Availability {
  if (!valid) return idle
  return {
    status: 'unavailable',
    message: `Could not check project ${field} availability. Try again.`,
  }
}

function initialAvailability(
  nameValid: boolean,
  slugValid: boolean,
  error?: unknown,
): ProjectIdentityAvailability {
  if (error)
    return {
      name: unavailable(nameValid, 'name'),
      slug: unavailable(slugValid, 'slug'),
    }
  return {
    name: checking(nameValid),
    slug: checking(slugValid),
  }
}

function sameAvailability(
  current: ProjectIdentityAvailability,
  next: ProjectIdentityAvailability,
) {
  return current.name.status === next.name.status
    && current.name.message === next.name.message
    && current.slug.status === next.slug.status
    && current.slug.message === next.slug.message
}

export function useProjectIdentityAvailability(
  name: string,
  slug: string,
  projects?: Project[],
  error?: unknown,
): ProjectIdentityAvailability {
  const trimmedName = name.trim()
  const normalizedName = normalize(name)
  const normalizedSlug = normalize(slug)
  const nameValid = isValidName(normalizedName)
  const slugValid = isValidSlug(slug)
  const projectsResolved = projects !== undefined
  const projectSignature = projects === undefined
    ? undefined
    : JSON.stringify(projects.map((project) => [normalize(project.name), normalize(project.slug)]))
  const existingProjects = useMemo(
    () => projectSignature ? JSON.parse(projectSignature) as Array<[string, string]> : [],
    [projectSignature],
  )
  const [availability, setAvailability] = useState(() =>
    initialAvailability(nameValid, slugValid, error),
  )

  useEffect(() => {
    if (error) {
      const next = {
        name: unavailable(nameValid, 'name'),
        slug: unavailable(slugValid, 'slug'),
      }
      setAvailability((current) => sameAvailability(current, next) ? current : next)
      return
    }

    if (!nameValid && !slugValid) {
      const next = { name: idle, slug: idle }
      setAvailability((current) => sameAvailability(current, next) ? current : next)
      return
    }

    const pending = {
      name: checking(nameValid),
      slug: checking(slugValid),
    }
    setAvailability((current) => sameAvailability(current, pending) ? current : pending)
    if (!projectsResolved) return

    const timer = window.setTimeout(() => {
      const next: ProjectIdentityAvailability = {
        name: !nameValid
          ? idle
          : existingProjects.some(([projectName]) => projectName === normalizedName)
            ? { status: 'conflict', message: `A project named “${trimmedName}” already exists` }
            : { status: 'available', message: 'Project name is available' },
        slug: !slugValid
          ? idle
          : existingProjects.some(([, projectSlug]) => projectSlug === normalizedSlug)
            ? { status: 'conflict', message: `The slug “${normalizedSlug}” is already in use` }
            : { status: 'available', message: 'Project slug is available' },
      }
      setAvailability((current) => sameAvailability(current, next) ? current : next)
    }, debounceMs)

    return () => window.clearTimeout(timer)
  }, [error, existingProjects, nameValid, normalizedName, normalizedSlug, projectsResolved, slugValid, trimmedName])

  return availability
}
