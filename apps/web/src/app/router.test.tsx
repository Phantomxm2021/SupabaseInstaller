import { QueryClient } from '@tanstack/react-query'
import { isValidElement } from 'react'
import { matchRoutes } from 'react-router-dom'
import { ConfigurationPage } from '../features/project/configuration/ConfigurationPage'
import { createAppRouter, NotFoundPage } from './router'

it('does not register removed project configuration compatibility routes', () => {
  const router = createAppRouter(new QueryClient())
  const shellRoute = router.routes.find((route) => route.children?.some((child) => child.path === '/projects/:projectId'))
  const projectRoute = shellRoute?.children?.find((route) => route.path === '/projects/:projectId')
  const childPaths = projectRoute?.children?.map((route) => route.path) ?? []
  expect(childPaths).toContain('configuration')
  expect(childPaths).not.toEqual(expect.arrayContaining(['services', 'authentication', 'database', 'storage', 'realtime', 'functions', 'pooler', 'network', 'secrets', 'settings']))
  expect(childPaths).toContain('*')
  const elementType = (element: unknown) => isValidElement(element) ? element.type : undefined
  expect(elementType(projectRoute?.children?.find((route) => route.path === '*')?.element)).toBe(NotFoundPage)
  expect(elementType(matchRoutes(router.routes, '/projects/bee/configuration')?.at(-1)?.route.element)).toBe(ConfigurationPage)
  for (const path of ['services', 'authentication', 'database', 'storage', 'realtime', 'functions', 'pooler', 'network', 'secrets', 'settings']) {
    expect(elementType(matchRoutes(router.routes, `/projects/bee/${path}`)?.at(-1)?.route.element)).toBe(NotFoundPage)
  }
  router.dispose()
})
