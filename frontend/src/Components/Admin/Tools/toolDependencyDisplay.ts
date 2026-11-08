import type { Tool } from '../../../api/tools'
import type { ToolMetadata } from '../../../api/toolMeta'
import { byLine } from './toolMetaDisplay'

// Pure display rules for the dependencies of a tool card and their modal (plan/ai/tools/step-217-tools-card-description-and-dependencies.md),
// kept apart from React so they can be script-checked. The verdicts mirror the install and enable check in core (manifest.ValidateRequiredTools):
// a dependency must be installed, enabled, and at least the required version, checked in that order.

// too_old: below the minimum; too_new: above the maximum the tool works with (nothing is ever downgraded by itself).
export type DependencyState = 'ready' | 'disabled' | 'too_old' | 'too_new' | 'missing'

export interface Requirement {
  slug: string
  minVersion: string
  maxVersion: string
}

type Installed = Pick<Tool, 'slug' | 'name' | 'version' | 'enabled' | 'status'> & { metadata: ToolMetadata | null }

// Dotted all-number versions ("0.0.12"); a missing trailing part counts as 0 ("1.2" equals "1.2.0"). null when either is not readable, as core
// refuses to compare such versions.
export function compareVersions(a: string, b: string): number | null {
  const split = (v: string): number[] | null => {
    const parts = v.split('.')
    const out: number[] = []
    for (const p of parts) {
      if (!/^\d+$/.test(p)) return null
      out.push(Number(p))
    }
    return out
  }
  const as = split(a)
  const bs = split(b)
  if (as === null || bs === null) return null
  for (let i = 0; i < Math.max(as.length, bs.length); i++) {
    const av = as[i] ?? 0
    const bv = bs[i] ?? 0
    if (av !== bv) return av < bv ? -1 : 1
  }
  return 0
}

export function dependencyState(req: Requirement, installed: Installed | undefined): DependencyState {
  if (!installed) return 'missing'
  if (!installed.enabled) return 'disabled'
  if (req.minVersion !== '') {
    const cmp = compareVersions(installed.version, req.minVersion)
    // an unreadable version cannot be shown to meet the minimum (core refuses it too)
    if (cmp === null || cmp < 0) return 'too_old'
  }
  if (req.maxVersion !== '') {
    const cmp = compareVersions(installed.version, req.maxVersion)
    if (cmp === null || cmp > 0) return 'too_new'
  }
  return 'ready'
}

export function requirementText(minVersion: string, maxVersion: string): string {
  if (minVersion !== '' && maxVersion !== '')
    return minVersion === maxVersion ? `Exactly version ${minVersion}` : `Version ${minVersion} to ${maxVersion}`
  if (minVersion !== '') return `Version ${minVersion} or newer`
  if (maxVersion !== '') return `Version ${maxVersion} or older`
  return 'Any version'
}

// The short form on a dependency pill: "any version", "≥ 0.0.1", "≤ 2.0.0", "0.0.1 – 2.0.0" or "= 1.0.0".
export function pillVersionText(minVersion: string, maxVersion: string): string {
  if (minVersion !== '' && maxVersion !== '') return minVersion === maxVersion ? `= ${minVersion}` : `${minVersion} – ${maxVersion}`
  if (minVersion !== '') return `≥ ${minVersion}`
  if (maxVersion !== '') return `≤ ${maxVersion}`
  return 'any version'
}

export interface DependencyView {
  slug: string
  // The installed tool's name, else the slug.
  name: string
  state: DependencyState
  needs: string
  // The short form for the pill (pillVersionText).
  pillVersion: string
  // "Version 0.0.123 · enabled · running", or "Not installed".
  installed: string
  description: string
  byline: string | null
  verdict: string
}

function installedText(i: Installed | undefined): string {
  if (!i) return 'Not installed'
  return `Version ${i.version} · ${i.enabled ? 'enabled' : 'disabled'} · ${i.status}`
}

function verdictFor(state: DependencyState, req: Requirement, installed: Installed | undefined): string {
  switch (state) {
    case 'ready':
      return 'Ready.'
    case 'disabled':
      return 'Installed but disabled: enable it on this page.'
    case 'too_old':
      return `Installed version ${installed?.version ?? ''} is older than the required ${req.minVersion}: update it.`
    case 'too_new':
      return `Installed version ${installed?.version ?? ''} is newer than the ${req.maxVersion} this tool works with: it is not used with it until this tool is updated.`
    case 'missing':
      return 'Not installed: install it from a tool repository.'
  }
}

export function describeDependency(req: Requirement, tools: readonly Installed[]): DependencyView {
  const installed = tools.find((t) => t.slug === req.slug)
  const state = dependencyState(req, installed)
  return {
    slug: req.slug,
    name: installed?.name ?? req.slug,
    state,
    needs: requirementText(req.minVersion, req.maxVersion),
    pillVersion: pillVersionText(req.minVersion, req.maxVersion),
    installed: installedText(installed),
    description: installed?.metadata?.description?.trim() ?? '',
    byline: installed ? byLine(installed.metadata) : null,
    verdict: verdictFor(state, req, installed),
  }
}

// How bad the worst dependency is, for the colour of the block: all ready, something to fix, or something missing.
export function worstState(states: readonly DependencyState[]): DependencyState {
  if (states.includes('missing')) return 'missing'
  if (states.includes('too_old')) return 'too_old'
  if (states.includes('too_new')) return 'too_new'
  if (states.includes('disabled')) return 'disabled'
  return 'ready'
}

export const stateLabel: Record<DependencyState, string> = {
  ready: 'Ready',
  disabled: 'Disabled',
  too_old: 'Needs update',
  too_new: 'Too new',
  missing: 'Not installed',
}

// The dot on a dependency pill: green, amber (installed but not usable as it is), red (not installed).
export const stateDot: Record<DependencyState, string> = {
  ready: 'bg-green-500',
  disabled: 'bg-amber-500',
  too_old: 'bg-amber-500',
  too_new: 'bg-amber-500',
  missing: 'bg-red-500',
}

// A short sentence for the block's tooltip.
export function blockTitle(toolName: string, views: readonly DependencyView[]): string {
  const bad = views.filter((v) => v.state !== 'ready')
  if (bad.length === 0)
    return `${toolName} needs ${views.length === 1 ? 'one other tool' : `${String(views.length)} other tools`}, and ${views.length === 1 ? 'it is' : 'all are'} ready. Click for details.`
  return `${bad.map((v) => v.name).join(', ')} ${bad.length === 1 ? 'needs' : 'need'} attention. Click for details.`
}
