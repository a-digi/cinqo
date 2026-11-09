import { useAuth } from '../Components/Auth/AuthContext'
import { formatBytes } from '../Shared/Components/Terminal/terminalRules'
import { SystemDetails } from './DetailsPanel'
import { MemoryTop } from './MemoryTop'
import { UnloadControl } from './UnloadControl'
import { canManageSystem, canUnload, isUnloadable } from './agentUnload'
import { AlertIcon, BotIcon, CheckIcon, PowerIcon, CpuIcon, CubeIcon, DiskIcon, MemoryIcon, SwapIcon } from './ResourceIcons'
import { agentView, pillText, pillTitle, resourceLevel, usedPercent, type AgentTone, type ResourceLevel } from './resourceLevel'
import { useFlash, useTweenedResources } from './tween'
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
  const { scopes } = useAuth()
  const shown = useTweenedResources(state.kind === 'ready' ? state.resources : null)
  if (state.kind === 'loading' || state.kind === 'hidden') return null
  if (state.kind === 'unknown' || shown === null) {
    return (
      <Shell gauge={null}>
        <div className="flex-1" />
        <Pill level="unknown" text="Memory: unknown" watch="unknown" title="The computer's memory could not be read." />
      </Shell>
    )
  }
  const r = shown
  const real = state.resources // levels, colours and the gauge follow the real reading; only the numbers glide
  const level = resourceLevel(real)
  const agent = agentView(r.agents)
  const agentWatch = agentView(real.agents).text
  return (
    <Shell gauge={{ level, percent: usedPercent(real) }}>
      <div className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden max-sm:hidden">
        <Chip
          icon={<CubeIcon />}
          label="Cinqo"
          value={formatBytes(r.cinqo.appBytes)}
          watch={formatBytes(real.cinqo.appBytes)}
          name="Cinqo memory"
        />
        {r.memory.swapUsedBytes > 0 && (
          <Chip
            icon={<SwapIcon />}
            label="Swap"
            value={formatBytes(r.memory.swapUsedBytes)}
            watch={formatBytes(real.memory.swapUsedBytes)}
            name="Swap in use"
          />
        )}
        {r.cpu.load1 >= 0 && r.cpu.cores > 0 && (
          <Chip
            icon={<CpuIcon />}
            label="CPU"
            value={`${r.cpu.load1.toFixed(1)} / ${r.cpu.cores}`}
            watch={`${real.cpu.load1.toFixed(1)} / ${real.cpu.cores}`}
            name={`CPU load ${r.cpu.load1.toFixed(1)} of ${r.cpu.cores} cores`}
          />
        )}
        {r.disk.totalBytes > 0 && (
          <Chip
            icon={<DiskIcon />}
            label="Disk"
            value={`${formatBytes(r.disk.freeBytes)} free`}
            watch={`${formatBytes(real.disk.freeBytes)} free`}
            name="Disk space"
          />
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <SystemDetails resources={r} canManage={canManageSystem(scopes)} />
        <AgentChip
          tone={agent.tone}
          text={agent.text}
          watch={agentWatch}
          unload={canUnload(scopes) && isUnloadable(agent.tone) ? { name: r.agents.find((a) => a.kind === 'local-ai')?.name ?? '' } : null}
        />
        <Pill level={level} text={pillText(r, level)} watch={pillText(real, level)} title={pillTitle(r)} interactive />
      </div>
    </Shell>
  )
}

const GAUGE: Record<ResourceLevel, string> = {
  unknown: 'bg-gray-300',
  ok: 'bg-emerald-500',
  orange: 'bg-orange-500',
  red: 'bg-red-500',
}

// unload: set when the person may unload and a model is loading, working or idle; the chip is then a button with a confirmation (plan 225).
function AgentChip({ tone, text, watch, unload }: { tone: AgentTone; text: string; watch: string; unload: { name: string } | null }) {
  const flash = useFlash(watch)
  const className = `inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ring-1 ring-inset transition-all duration-500 ${AGENT[tone].chip} ${flash ? 'scale-105 shadow-md' : ''}`
  const inside = (
    <>
      <BotIcon />
      <span aria-hidden="true" className={`h-1.5 w-1.5 rounded-full ${AGENT[tone].dot}`} />
      <span className="max-md:hidden">{text}</span>
      {unload && (
        <span
          aria-hidden="true"
          data-testid="unload-badge"
          className="-mr-1 ml-0.5 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-red-600 text-white shadow-sm ring-1 ring-red-700/30"
        >
          <PowerIcon />
        </span>
      )}
    </>
  )
  if (unload) {
    return (
      <UnloadControl tone={tone} name={unload.name} label={`Local agent: ${text}. Click to unload.`} className={className}>
        {inside}
      </UnloadControl>
    )
  }
  return (
    <span role="group" aria-label={`Local agent: ${text}`} title={text} className={className}>
      {inside}
    </span>
  )
}

function Shell({ children, gauge }: { children: React.ReactNode; gauge: { level: ResourceLevel; percent: number } | null }) {
  return (
    <div
      role="status"
      aria-label="Computer resources"
      className="relative flex h-11 shrink-0 items-center gap-3 border-b border-slate-200 bg-gradient-to-r from-slate-100 via-slate-50 to-slate-100 px-4 text-xs text-slate-600 shadow-inner"
    >
      {children}
      {gauge && (
        <div aria-hidden="true" className="absolute inset-x-0 bottom-0 h-0.5 bg-slate-200">
          <div
            className={`h-full transition-[width,background-color] duration-700 ease-out ${GAUGE[gauge.level]}`}
            style={{ width: `${gauge.percent}%` }}
          />
        </div>
      )}
    </div>
  )
}

function Chip({
  icon,
  label,
  value,
  watch,
  name,
  tone,
}: {
  icon: React.ReactNode
  label: string
  value: string
  watch: string
  name: string
  tone?: string
}) {
  const flash = useFlash(watch)
  return (
    <span
      role="group"
      aria-label={`${name}: ${value}`}
      className={`inline-flex shrink-0 items-center gap-1.5 rounded-md px-2 py-1 shadow-sm ring-1 ring-inset transition-all duration-500 ${flash ? 'bg-sky-50 text-slate-800 ring-sky-300' : 'bg-white text-slate-700 ring-slate-200'} ${tone ?? ''}`}
    >
      <span className="text-slate-400">{icon}</span>
      {label !== '' && <span className="text-slate-500">{label}</span>}
      <span className="font-semibold">{value}</span>
    </span>
  )
}

// interactive: the pill is a button that drops down the applications that use the most memory (plan 226). Not for the grey "unknown" pill, where the
// list could not be read either.
function Pill({
  level,
  text,
  watch,
  title,
  interactive,
}: {
  level: ResourceLevel
  text: string
  watch: string
  title: string
  interactive?: boolean
}) {
  const flash = useFlash(watch)
  const className = `inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold ring-1 ring-inset transition-all duration-500 ${PILL[level]} ${flash ? 'scale-105 shadow-md' : ''}`
  const inside = (
    <>
      {level === 'orange' || level === 'red' ? <AlertIcon /> : level === 'ok' ? <CheckIcon /> : <MemoryIcon />}
      <span>{text}</span>
    </>
  )
  if (interactive) {
    return (
      <MemoryTop label={`${text}. Click to see which programs use the memory.`} title={title} className={className}>
        {inside}
      </MemoryTop>
    )
  }
  return (
    <span title={title} className={className}>
      {inside}
    </span>
  )
}
