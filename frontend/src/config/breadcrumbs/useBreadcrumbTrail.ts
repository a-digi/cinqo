import { useMemo } from 'react'
import { useLocation } from 'react-router-dom'
import { useBreadcrumbRegistry } from './BreadcrumbRegistryContext'
import { applyLabels, resolveBreadcrumbs, type BreadcrumbItem } from './breadcrumbRules'

// The trail for the page the person is on: looked up again on every address change and whenever a tool registers one
// (so a trail appears as soon as its tool's bundle has loaded), with the labels the page itself set (step 7) put in. Empty = nothing registered for this page = no bar.
export function useBreadcrumbTrail(): BreadcrumbItem[] {
  const { pathname, search } = useLocation()
  const { registrations, labelState } = useBreadcrumbRegistry()
  return useMemo(
    () => applyLabels(resolveBreadcrumbs(registrations, pathname, search), labelState, pathname + search),
    [registrations, labelState, pathname, search],
  )
}
