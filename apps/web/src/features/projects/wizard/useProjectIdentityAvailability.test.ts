import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Project } from '../../../api/types'
import { useProjectIdentityAvailability } from './useProjectIdentityAvailability'

const project = (name: string, slug: string) => ({
  id: slug,
  name,
  slug,
} as Project)

afterEach(() => {
  vi.useRealTimers()
})

describe('useProjectIdentityAvailability', () => {
  it('reports independent name and slug conflicts after 400ms', async () => {
    vi.useFakeTimers()
    const projects = [project('Production API', 'production-api')]
    const { result } = renderHook(() =>
      useProjectIdentityAvailability(' Production API ', 'production-api', projects),
    )

    expect(result.current.name.status).toBe('checking')
    expect(result.current.slug.status).toBe('checking')

    act(() => {
      vi.advanceTimersByTime(400)
    })

    expect(result.current.name).toMatchObject({
      status: 'conflict',
      message: 'A project named “Production API” already exists',
    })
    expect(result.current.slug).toMatchObject({
      status: 'conflict',
      message: 'The slug “production-api” is already in use',
    })
  })

  it('reports a lookup error as unavailable, not a conflict', () => {
    const { result } = renderHook(() =>
      useProjectIdentityAvailability(
        'Production API',
        'production-api',
        undefined,
        new Error('offline'),
      ),
    )

    expect(result.current.name).toMatchObject({ status: 'unavailable' })
    expect(result.current.slug).toMatchObject({ status: 'unavailable' })
  })

  it('keeps valid identities checking while the project list is unresolved', () => {
    vi.useFakeTimers()
    const { result } = renderHook(() =>
      useProjectIdentityAvailability('Development API', 'development-api'),
    )

    act(() => {
      vi.advanceTimersByTime(400)
    })

    expect(result.current.name.status).toBe('checking')
    expect(result.current.slug.status).toBe('checking')
  })

  it('returns idle for empty or locally invalid identities', () => {
    const { result } = renderHook(() =>
      useProjectIdentityAvailability('   ', 'PRODUCTION-API', []),
    )

    expect(result.current.name.status).toBe('idle')
    expect(result.current.slug.status).toBe('idle')
  })

  it('reports independently available valid identities after 400ms', async () => {
    vi.useFakeTimers()
    const { result } = renderHook(() =>
      useProjectIdentityAvailability('Development API', 'development-api', [project('Production API', 'production-api')]),
    )

    act(() => {
      vi.advanceTimersByTime(400)
    })

    expect(result.current.name).toMatchObject({ status: 'available' })
    expect(result.current.slug).toMatchObject({ status: 'available' })
  })
})
