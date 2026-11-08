import type { PlanStep, PlanStepStatus, SubAgentRun } from '../../api/conversations'
import { SubAgentRow } from './SubAgentRow'

// The composer orchestrator's plan (composer step 7): every run_plan
// step with its dependencies and status, each linked to the worker
// sub-agent that ran it (that worker's result, tokens and failure
// reason come from the same SubAgentRow the flat sub-agents list
// uses). A step with no worker yet — still waiting on a dependency —
// or none ever, because something it depended on failed, shows as a
// plain row. Steps are grouped by plan number only when the
// orchestrator re-planned. See
// plan/ai/conversation/step-42-orchestrator-composer-mode.md.
const STEP_DOT_CLASSES: Record<PlanStepStatus, string> = {
  pending: 'bg-gray-300',
  running: 'animate-pulse bg-amber-500',
  completed: 'bg-green-500',
  failed: 'bg-red-500',
  skipped: 'border border-gray-300 bg-white',
}

const STEP_STATUS_LABEL: Partial<Record<PlanStepStatus, string>> = {
  pending: 'waiting',
  skipped: 'skipped — a step it depends on did not complete',
}

export function PlanPanel({ plan, subAgents }: { plan: PlanStep[]; subAgents: SubAgentRun[] }) {
  const workers = new Map(subAgents.map((a) => [a.id, a]))
  const planNumbers = [...new Set(plan.map((s) => s.planNo))]
  const working = plan.some((s) => s.status === 'running' || s.status === 'pending')

  return (
    <div className="space-y-1">
      <p className="font-medium text-gray-500">{working ? 'Working on the plan…' : 'Plan'}</p>
      {planNumbers.map((planNo) => (
        <div key={planNo} className="space-y-1">
          {planNumbers.length > 1 && <p className="pt-1 text-gray-400">Plan {planNo}</p>}
          {plan
            .filter((s) => s.planNo === planNo)
            .map((step) => {
              const worker = step.subAgentId ? workers.get(step.subAgentId) : undefined
              return worker ? (
                <SubAgentRow key={`${step.planNo}-${step.id}`} agent={worker} step={step} />
              ) : (
                <PlannedStepRow key={`${step.planNo}-${step.id}`} step={step} />
              )
            })}
        </div>
      ))}
    </div>
  )
}

function PlannedStepRow({ step }: { step: PlanStep }) {
  return (
    <div className="flex items-start gap-1.5 text-xs text-gray-400">
      <span className={`mt-1 h-1.5 w-1.5 shrink-0 rounded-full ${STEP_DOT_CLASSES[step.status]}`} />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-gray-500">
          <span className="mr-1 font-medium text-gray-600">{step.id}</span>
          {step.task}
        </span>
        {step.dependsOn.length > 0 && <span className="block">after {step.dependsOn.join(', ')}</span>}
        {STEP_STATUS_LABEL[step.status] && <span className="block">{STEP_STATUS_LABEL[step.status]}</span>}
      </span>
    </div>
  )
}
