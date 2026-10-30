import { Fragment, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import type { AllAiTraceLogsEntry } from '../../../api/conversations'
import { Pill } from '../../../Shared/Components/Pill/Pill'
import { useSnackBar } from '../../../Shared/Components/SnackBar/SnackBarContext'
import { groupByDay, hasTitle, pluralFiles, relativeTime, shortFolder, shortId, traceKind, type SortKey } from './aiLogsRules'

// The AI Logs overview table (plan/ai/conversation/step-43-ai-logs-table-redesign.md): sortable headers,
// day groups while sorted by date, a short copyable id and folder, relative dates with the exact time on
// hover, loading / empty / no-match states, and stacked cards under `md`. Purely presentational: the page
// owns the data, the filter and the sort.
export interface AiLogsTableProps {
  // Already filtered and sorted.
  entries: AllAiTraceLogsEntry[]
  loading: boolean
  // The active search text, only used to tell "nothing yet" from "nothing matches".
  query: string
  onClearQuery: () => void
  sortKey: SortKey
  onSortChange: (key: SortKey) => void
  now: Date
}

function CopyIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.5" className="h-3.5 w-3.5" aria-hidden="true">
      <rect x="7" y="7" width="9" height="9" rx="1.5" />
      <path d="M13 7V5.5A1.5 1.5 0 0 0 11.5 4h-6A1.5 1.5 0 0 0 4 5.5v6A1.5 1.5 0 0 0 5.5 13H7" strokeLinecap="round" />
    </svg>
  )
}

// Copies what the row already shows in full (the id or the folder). The clipboard can be refused (an
// insecure origin, a blocked permission), so a failure says so instead of staying silent.
function CopyButton({ text, label }: { text: string; label: string }) {
  const { successMessage, errorMessage } = useSnackBar()
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={() => {
        navigator.clipboard.writeText(text).then(
          () => {
            successMessage('Copied')
          },
          () => {
            errorMessage('Could not copy. Select the text and copy it by hand.')
          },
        )
      }}
      className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-gray-400 transition hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
    >
      <CopyIcon />
    </button>
  )
}

function Title({ entry }: { entry: AllAiTraceLogsEntry }) {
  const kind = traceKind(entry.title)
  if (!hasTitle(entry)) {
    return (
      <div className="flex items-center gap-2">
        <span className="truncate text-sm italic text-gray-500">Deleted conversation</span>
        <Pill>Deleted</Pill>
      </div>
    )
  }
  return (
    <div className="flex min-w-0 items-center gap-2">
      {kind && (
        <span className="shrink-0 whitespace-nowrap">
          <Pill outline>{kind}</Pill>
        </span>
      )}
      <span className="truncate text-sm font-medium text-gray-900" title={entry.title}>
        {entry.title}
      </span>
    </div>
  )
}

function Id({ id }: { id: string }) {
  return (
    <div className="mt-0.5 flex items-center gap-1 font-mono text-xs text-gray-500">
      <span title={id}>{shortId(id)}</span>
      <CopyButton text={id} label="Copy conversation id" />
    </div>
  )
}

function FilesPill({ count }: { count: number }) {
  return <Pill variant={count > 1 ? 'green' : 'gray'}>{pluralFiles(count)}</Pill>
}

function When({ iso, now }: { iso: string; now: Date }) {
  const date = new Date(iso)
  const valid = !Number.isNaN(date.getTime())
  return (
    <time
      dateTime={valid ? date.toISOString() : undefined}
      title={valid ? date.toLocaleString() : undefined}
      className="text-sm text-gray-600"
    >
      {relativeTime(iso, now)}
    </time>
  )
}

function Folder({ path }: { path: string }) {
  return (
    <div className="flex min-w-0 items-center gap-1">
      <span className="truncate font-mono text-xs text-gray-600" title={path}>
        {shortFolder(path)}
      </span>
      <CopyButton text={path} label="Copy folder path" />
    </div>
  )
}

function SortHeader({
  label,
  active,
  direction,
  onClick,
}: {
  label: string
  active: boolean
  direction: 'ascending' | 'descending'
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`inline-flex items-center gap-1 rounded text-xs font-semibold uppercase tracking-wide focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
        active ? 'text-gray-900' : 'text-gray-500 hover:text-gray-800'
      }`}
    >
      {label}
      <span aria-hidden="true" className={active ? 'text-gray-900' : 'text-gray-300'}>
        {active ? (direction === 'ascending' ? '↑' : '↓') : '↕'}
      </span>
    </button>
  )
}

function SkeletonRows() {
  return (
    <div className="animate-pulse divide-y divide-gray-100" aria-hidden="true">
      {Array.from({ length: 6 }, (_, i) => (
        <div key={i} className="flex items-center gap-6 px-5 py-4">
          <div className="flex-1 space-y-2">
            <div className="h-3.5 w-2/3 rounded bg-gray-200" />
            <div className="h-3 w-24 rounded bg-gray-100" />
          </div>
          <div className="h-5 w-16 rounded-full bg-gray-100 max-md:hidden" />
          <div className="h-3.5 w-24 rounded bg-gray-100 max-md:hidden" />
          <div className="h-3.5 w-40 rounded bg-gray-100 max-lg:hidden" />
        </div>
      ))}
    </div>
  )
}

function Message({ children }: { children: ReactNode }) {
  return <div className="px-6 py-14 text-center text-sm text-gray-500">{children}</div>
}

export function AiLogsTable({ entries, loading, query, onClearQuery, sortKey, onSortChange, now }: AiLogsTableProps) {
  const byDate = sortKey === 'newest' || sortKey === 'oldest'
  const groups = byDate ? groupByDay(entries, now) : [{ label: '', entries }]
  const ariaSort = (key: SortKey | SortKey[]): 'ascending' | 'descending' | 'none' => {
    const keys = Array.isArray(key) ? key : [key]
    if (!keys.includes(sortKey)) return 'none'
    return sortKey === 'oldest' || sortKey === 'title' ? 'ascending' : 'descending'
  }

  return (
    <div className="overflow-clip rounded-xl border border-gray-200 bg-white shadow-sm">
      {loading ? (
        <SkeletonRows />
      ) : entries.length === 0 ? (
        query.trim() !== '' ? (
          <Message>
            Nothing matches “{query.trim()}”.{' '}
            <button type="button" onClick={onClearQuery} className="font-medium text-blue-600 hover:underline">
              Clear search
            </button>
          </Message>
        ) : (
          <Message>
            No conversation has produced an AI trace log yet.{' '}
            <Link to="/admin/conversation-debug" className="font-medium text-blue-600 hover:underline">
              Turn trace logging on in Settings
            </Link>
            .
          </Message>
        )
      ) : (
        <>
          <table className="w-full table-fixed text-left max-md:hidden">
            <thead className="sticky top-0 z-10 bg-gray-50/95 backdrop-blur">
              <tr className="border-b border-gray-200">
                <th scope="col" aria-sort={ariaSort('title')} className="w-[42%] px-5 py-3">
                  <SortHeader
                    label="Conversation"
                    active={sortKey === 'title'}
                    direction="ascending"
                    onClick={() => {
                      onSortChange('title')
                    }}
                  />
                </th>
                <th scope="col" aria-sort={ariaSort('files')} className="w-[12%] px-3 py-3">
                  <SortHeader
                    label="Files"
                    active={sortKey === 'files'}
                    direction="descending"
                    onClick={() => {
                      onSortChange('files')
                    }}
                  />
                </th>
                <th scope="col" aria-sort={ariaSort(['newest', 'oldest'])} className="w-[20%] px-3 py-3">
                  <SortHeader
                    label="Last modified"
                    active={byDate}
                    direction={sortKey === 'oldest' ? 'ascending' : 'descending'}
                    onClick={() => {
                      onSortChange(sortKey === 'newest' ? 'oldest' : 'newest')
                    }}
                  />
                </th>
                <th scope="col" className="w-[26%] px-3 py-3 pr-5 text-xs font-semibold uppercase tracking-wide text-gray-500">
                  Folder
                </th>
              </tr>
            </thead>
            <tbody>
              {groups.map((group) => (
                <Fragment key={group.label || 'all'}>
                  {group.label !== '' && (
                    <tr className="bg-gray-50/60">
                      <th scope="colgroup" colSpan={4} className="px-5 py-1.5 text-left text-xs font-semibold text-gray-500">
                        {group.label}
                      </th>
                    </tr>
                  )}
                  {group.entries.map((e) => (
                    <tr
                      key={e.conversationId}
                      className="border-t border-gray-100 transition-colors hover:bg-blue-50/40 focus-within:bg-blue-50/40"
                    >
                      <td className="min-w-0 px-5 py-3">
                        <Title entry={e} />
                        <Id id={e.conversationId} />
                      </td>
                      <td className="px-3 py-3">
                        <FilesPill count={e.logCount} />
                      </td>
                      <td className="px-3 py-3">
                        <When iso={e.modifiedAt} now={now} />
                      </td>
                      <td className="min-w-0 px-3 py-3 pr-5">
                        <Folder path={e.folder} />
                      </td>
                    </tr>
                  ))}
                </Fragment>
              ))}
            </tbody>
          </table>

          <div className="md:hidden">
            {groups.map((group) => (
              <section key={group.label || 'all'} aria-label={group.label || undefined}>
                {group.label !== '' && <h3 className="bg-gray-50 px-4 py-1.5 text-xs font-semibold text-gray-500">{group.label}</h3>}
                <ul className="divide-y divide-gray-100">
                  {group.entries.map((e) => (
                    <li key={e.conversationId} className="space-y-2 px-4 py-3">
                      <Title entry={e} />
                      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                        <FilesPill count={e.logCount} />
                        <When iso={e.modifiedAt} now={now} />
                      </div>
                      <Id id={e.conversationId} />
                      <Folder path={e.folder} />
                    </li>
                  ))}
                </ul>
              </section>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
