import type {
  DoneStep,
  InstallPlan,
  InstallPlanAlternative,
  InstallPlanProblem,
  InstallPlanStep,
  InstallRun,
  PlanMark,
} from '../../../api/toolRepositories'

// Pure display rules of the install-with-dependencies dialog (plan/ai/tools/repository/dependency-install.md, step 7), kept apart from React so they
// can be script-checked.

export function stepTitle(s: InstallPlanStep): string {
  switch (s.action) {
    case 'install':
      return `Install ${s.name} v${s.version}`
    case 'update':
      return s.installedVersion ? `Update ${s.name} v${s.installedVersion} → v${s.version}` : `Update ${s.name} to v${s.version}`
    case 'enable':
      return `Enable ${s.name}`
  }
}

// Why a step is in the list.
export function stepReason(s: InstallPlanStep, rootId: string): string {
  if (s.toolId === rootId) return ''
  if (s.requiredBy.length === 0) return ''
  return `needed by ${s.requiredBy.join(', ')}`
}

export function markText(mark: PlanMark, s: InstallPlanStep): string {
  switch (mark) {
    case 'other_repository':
      return `Comes from another repository: ${s.repositoryName}`
    case 'local_network':
      return `${s.repositoryName} is on a local network address`
    case 'stale_repository':
      return `${s.repositoryName} could not be refreshed: its list may be out of date`
    case 'restarts':
      return s.dependents.length > 0 ? `Restarts, and ${s.dependents.join(', ')} depends on it` : 'Restarts'
    case 'was_disabled':
      return 'Is disabled now and is enabled afterwards'
  }
}

const problemTitles: Record<string, string> = {
  not_in_any_repository: 'Not available in any repository',
  no_version_in_range: 'No version fits',
  would_break_installed: 'Would break an installed tool',
  needs_downgrade: 'Would need an older version',
  conflicting_requirements: 'Tools disagree on the version',
  not_installable_here: 'Cannot be installed here',
  publisher_changed: 'Another publisher',
  conflicting_copies: 'Two repositories disagree',
  stale_only_source: 'Only an out-of-date repository has it',
  cycle: 'The tools need each other',
  too_deep: 'Too many levels of dependencies',
  too_many: 'Too many tools at once',
  already_installed: 'Already installed',
  older_than_installed: 'Older than the installed version',
  installed_version_unreadable: 'The installed version cannot be read',
  unstable: 'The packages keep changing what they need',
}

export function problemTitle(p: InstallPlanProblem): string {
  return problemTitles[p.kind] ?? 'Cannot be installed'
}

// A tool from a repository other than the one being looked at needs its own tick before the plan can be confirmed.
export function needsTick(s: InstallPlanStep): boolean {
  return s.marks.includes('other_repository')
}

export function ticksNeeded(plan: InstallPlan): string[] {
  return plan.steps.filter(needsTick).map((s) => s.toolId)
}

export function canConfirm(plan: InstallPlan, ticked: ReadonlySet<string>): boolean {
  return plan.ok && plan.steps.length > 0 && plan.problems.length === 0 && ticksNeeded(plan).every((id) => ticked.has(id))
}

// Nothing to explain: one step, no marks, no problems. The page then asks the plain question as before.
export function isSimple(plan: InstallPlan): boolean {
  return plan.ok && plan.steps.length === 1 && plan.steps[0].marks.length === 0 && plan.problems.length === 0
}

export function sizeText(bytes: number): string {
  if (bytes <= 0) return 'size unknown'
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function names(steps: DoneStep[]): string {
  return steps.map((s) => `${s.action} ${s.toolId} ${s.version}`.trim()).join(', ')
}

// The line shown after a run that did not finish.
export function runText(run: InstallRun): string {
  switch (run.code) {
    case 'step_failed':
      return `${run.message}. Done: ${run.installed.length === 0 ? 'nothing' : names(run.installed)}. Not done: ${names(run.notDone)}.`
    case 'plan_changed':
      return 'What has to be installed changed while it was being checked. Please look at the list again.'
    case 'plan_blocked':
      return run.message
    default:
      return ''
  }
}

// The offer made when an installed tool holds the requested one back: update that tool instead, and the requested one comes along.
export function alternativeText(a: InstallPlanAlternative): string {
  return `Update ${a.name} to v${a.version} instead: that version works with the newer ${a.forToolId}, and brings it along.`
}
