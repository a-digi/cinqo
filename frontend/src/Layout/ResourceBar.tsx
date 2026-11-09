import { formatBytes } from '../Shared/Components/Terminal/terminalRules'
import { AlertIcon, BotIcon, CheckIcon, CpuIcon, CubeIcon, DiskIcon, LaptopIcon, MemoryIcon, SwapIcon } from './ResourceIcons'
import { agentView, deviceText, pillText, pillTitle, resourceLevel, type AgentTone, type ResourceLevel } from './resourceLevel'
import { useResources } from './useResources'

// The pill's colours: strong enough to be noticed at a glance, and never the only sign (its words and icon change too).
const PILL: Record<ResourceLevel, string> = {
  unknown: 'bg-gray-100 text-gray-600 ring-gray-200',
  ok: 'bg-emerald-50 text-emerald-800 ring-emerald-200',
  orange: 'bg-orange-100 text-orange-900 ring-orange-300',
  red: 'bg-red-100 text-red-900 ring-red-300',
}

const AGENT: Record<AgentTone, { chip: string; dot: string }> = {
  none: { chip: 'bg-white text-gray-500 ring-gray-200', dot: 'bg-gray-300' },
  starting: { chip: 'bg-amber-50 text-amber-900 ring-amber-200', dot: 'bg-amber-500 animate-pulse' },
  working: { chip: 'bg-teal-50 text-teal-900 ring-teal-200', dot: 'bg-teal-500 animate-pulse' },
  idle: { chip: 'bg-emerald-50 text-emerald-900 ring-emerald-200', dot: 'bg-emerald-500' },
  failed: { chip: 'bg-red-50 text-red-900 ring-red-200', dot: 'bg-red-500' },
}

// The bar under the top bar (plan 222, 223): what the computer is and what it is doing, always visible, on its own soft background. Every
// figure is a small chip with an icon. The pill at the right turns orange or red when the memory is tight, and the agent chip says whether a
// local agent is running. Hidden by width with max-* variants (never "hidden" plus a breakpoint: tool stylesheets load their own `.hidden`).
export function ResourceBar() {
  const state = useResources()
  if (state.kind === 'loading' || state.kind === 'hidden') return null
  if (state.kind === 'unknown') {
    return (
      <Shell>
        <div className="flex-1" />
        <Pill level="unknown" text="Memory: unknown" title="The computer's memory could not be read." />
      </Shell>
    )
  }
  const r = state.resources
  const level = resourceLevel(r)
  const device = deviceText(r)
  const agent = agentView(r.agents)
  return (
    <Shell>
      <div className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden max-sm:hidden">
        {device !== '' && (
          <Chip
            icon={<LaptopIcon />}
            label=""
            value={device}
            name="Device"
            tone="text-slate-700 ring-slate-200 bg-white font-medium max-lg:hidden"
          />
        )}
        <Chip icon={<CubeIcon />} label="Cinqo" value={formatBytes(r.cinqo.appBytes)} name="Cinqo memory" />
        {r.memory.swapUsedBytes > 0 && (
          <Chip icon={<SwapIcon />} label="Swap" value={formatBytes(r.memory.swapUsedBytes)} name="Swap in use" />
        )}
        {r.cpu.load1 >= 0 && r.cpu.cores > 0 && (
          <Chip
            icon={<CpuIcon />}
            label="CPU"
            value={`${r.cpu.load1.toFixed(1)} / ${r.cpu.cores}`}
            name={`CPU load ${r.cpu.load1.toFixed(1)} of ${r.cpu.cores} cores`}
          />
        )}
        {r.disk.totalBytes > 0 && (
          <Chip icon={<DiskIcon />} label="Disk" value={`${formatBytes(r.disk.freeBytes)} free`} name="Disk space" />
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <span
          role="group"
          aria-label={`Local agent: ${agent.text}`}
          title={agent.text}
          className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ring-1 ring-inset ${AGENT[agent.tone].chip}`}
        >
          <BotIcon />
          <span aria-hidden="true" className={`h-1.5 w-1.5 rounded-full ${AGENT[agent.tone].dot}`} />
          <span className="max-md:hidden">{agent.text}</span>
        </span>
        <Pill level={level} text={pillText(r, level)} title={pillTitle(r)} />
      </div>
    </Shell>
  )
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div
      role="status"
      aria-label="Computer resources"
      className="flex h-11 shrink-0 items-center gap-3 border-b border-slate-200 bg-gradient-to-r from-slate-100 via-slate-50 to-slate-100 px-4 text-xs text-slate-600 shadow-inner"
    >
      {children}
    </div>
  )
}

function Chip({ icon, label, value, name, tone }: { icon: React.ReactNode; label: string; value: string; name: string; tone?: string }) {
  return (
    <span
      role="group"
      aria-label={`${name}: ${value}`}
      className={`inline-flex shrink-0 items-center gap-1.5 rounded-md px-2 py-1 shadow-sm ring-1 ring-inset ${tone ?? 'bg-white text-slate-700 ring-slate-200'}`}
    >
      <span className="text-slate-400">{icon}</span>
      {label !== '' && <span className="text-slate-500">{label}</span>}
      <span className="font-semibold">{value}</span>
    </span>
  )
}

function Pill({ level, text, title }: { level: ResourceLevel; text: string; title: string }) {
  return (
    <span
      title={title}
      className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold ring-1 ring-inset ${PILL[level]}`}
    >
      {level === 'orange' || level === 'red' ? <AlertIcon /> : level === 'ok' ? <CheckIcon /> : <MemoryIcon />}
      <span>{text}</span>
    </span>
  )
}
