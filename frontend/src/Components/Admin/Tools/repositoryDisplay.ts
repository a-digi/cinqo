import type { RepositoryCounts, RepositoryTool, ToolRepository, ToolStatus } from '../../../api/toolRepositories'

// Pure display rules for the Repositories page, kept apart from React so they
// can be checked with a script. See plan/ai/tools/repository/tool-repositories.md.

type PillVariant = 'gray' | 'green' | 'amber' | 'red'

export const statusLabel: Record<ToolStatus, string> = {
  not_installed: 'Not installed',
  up_to_date: 'Up to date',
  update_available: 'Update available',
  newer_than_repository: 'Newer than repository',
  unavailable: 'Unavailable',
}

export const statusVariant: Record<ToolStatus, PillVariant> = {
  not_installed: 'gray',
  up_to_date: 'green',
  update_available: 'amber',
  newer_than_repository: 'gray',
  unavailable: 'red',
}

export type ToolAction = 'install' | 'update'

// Only a tool that can actually be installed or updated gets a button; an
// unavailable one explains itself through its reason instead.
export function actionFor(tool: Pick<RepositoryTool, 'status'>): ToolAction | null {
  if (tool.status === 'not_installed') return 'install'
  if (tool.status === 'update_available') return 'update'
  return null
}

function plural(n: number, one: string, many: string): string {
  return n === 1 ? one : many
}

// "2 of 5 installed tools are up to date; 2 can be updated"
export function summaryText(counts: RepositoryCounts): string {
  if (counts.tools === 0) return 'This repository lists no tools.'
  if (counts.installed === 0) {
    return `None of its ${counts.tools} ${plural(counts.tools, 'tool is', 'tools are')} installed.`
  }
  const upToDate = `${counts.upToDate} of ${counts.installed} installed ${plural(counts.installed, 'tool is', 'tools are')} up to date`
  if (counts.updatable === 0) return `${upToDate}.`
  return `${upToDate}; ${counts.updatable} can be updated.`
}

// The repository table's one-line summary: "3 tools · 1 update available".
export function countsCell(counts: RepositoryCounts): string {
  if (counts.tools === 0) return 'No tools'
  const tools = `${counts.tools} ${plural(counts.tools, 'tool', 'tools')}`
  if (counts.updatable === 0) return tools
  return `${tools} · ${counts.updatable} ${plural(counts.updatable, 'update', 'updates')} available`
}

export function repositoryPath(id: string): string {
  return `/admin/tool-repositories/${encodeURIComponent(id)}`
}

export function repositoryStatusVariant(status: ToolRepository['lastStatus']): PillVariant {
  if (status === 'ok') return 'green'
  if (status === 'error') return 'red'
  return 'gray'
}

export function repositoryStatusLabel(status: ToolRepository['lastStatus']): string {
  if (status === 'ok') return 'Loaded'
  if (status === 'error') return 'Refresh failed'
  return 'Not loaded yet'
}

// The backend stamps UTC as "2026-10-30 12:00:00"; an unreadable value is
// shown as it is rather than hidden.
export function formatFetched(value: string): string {
  if (!value) return 'never'
  const d = new Date(value.replace(' ', 'T') + 'Z')
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString()
}

export function installConfirmMessage(
  tool: Pick<RepositoryTool, 'name' | 'installedVersion' | 'currentVersion'>,
  repository: Pick<ToolRepository, 'name' | 'url'>,
): string {
  const what =
    tool.installedVersion !== ''
      ? `Update "${tool.name}" from ${tool.installedVersion} to ${tool.currentVersion}`
      : `Install "${tool.name}" ${tool.currentVersion}`
  return `${what} from "${repository.name}" (${repository.url})? A tool is a program that runs on this computer with this app's permissions. Only continue if you trust this repository.`
}

export function addLinkProblem(link: string): string | null {
  const value = link.trim()
  if (value === '') return 'Enter the link to the repository file.'
  if (!/^https?:\/\//i.test(value)) return 'The link must start with https:// (or http:// for a local address).'
  return null
}
