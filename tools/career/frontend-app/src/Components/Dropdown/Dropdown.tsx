import { useEffect, useId, useRef, useState } from 'react'

export interface DropdownOption {
  value: string
  label: string
}

type DropdownProps =
  | {
      multiple?: false
      options: DropdownOption[]
      value: string | null
      onChange: (value: string) => void
      label?: string
      placeholder?: string
      searchable?: boolean
    }
  | {
      multiple: true
      options: DropdownOption[]
      value: string[]
      onChange: (value: string[]) => void
      label?: string
      placeholder?: string
      searchable?: boolean
    }

// Shared single/multiple-select dropdown — a real popover listbox,
// not a native <select>, so multiple mode gets real checkbox-style
// UX (toggle a row, popover stays open for the next pick) instead of
// native <select multiple>'s own ctrl/cmd-click behavior with no
// visible "currently selected" summary. A discriminated union on
// `multiple` gives every call site a correctly-typed `value`/
// `onChange` (string vs. string[]) with no cast needed. See
// plan/ai/tools/career/step-13-dropdown-component.md.
//
// `searchable` (step 54) adds a text input at the top of the open
// popover that filters `options` by label — opt-in and fully
// backward-compatible, every existing call site behaves exactly as
// before unless it passes `searchable`. See
// plan/ai/tools/career/step-54-jobs-page-searchable-dropdown-filters-toggle.md.
export function Dropdown(props: DropdownProps) {
  const { options, label, placeholder = 'Select…', searchable = false } = props
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const id = useId()

  useEffect(() => {
    if (!open) return
    function handleClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [open])

  useEffect(() => {
    if (!open) {
      setSearch('')
      return
    }
    if (searchable) searchInputRef.current?.focus()
  }, [open, searchable])

  function isSelected(value: string): boolean {
    if (props.multiple) return props.value.includes(value)
    return props.value === value
  }

  function handleSelect(value: string) {
    if (props.multiple) {
      const current = props.value
      const next = current.includes(value) ? current.filter((v) => v !== value) : [...current, value]
      props.onChange(next)
      return
    }
    props.onChange(value)
    setOpen(false)
  }

  const selectedLabels = options.filter((o) => isSelected(o.value)).map((o) => o.label)
  const triggerText = selectedLabels.length > 0 ? selectedLabels.join(', ') : placeholder
  const visibleOptions = searchable ? options.filter((o) => o.label.toLowerCase().includes(search.toLowerCase())) : options

  return (
    <div ref={containerRef} className="relative inline-block text-sm">
      {label && (
        <label id={`${id}-label`} className="mb-1 block text-xs font-medium text-gray-500">
          {label}
        </label>
      )}
      <button
        type="button"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-labelledby={label ? `${id}-label` : undefined}
        onClick={() => {
          setOpen((prev) => !prev)
        }}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown' && !open) {
            e.preventDefault()
            setOpen(true)
          }
        }}
        className="flex min-w-40 items-center justify-between gap-2 rounded-md border border-gray-300 px-2 py-1 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
      >
        <span className="truncate">{triggerText}</span>
        <CaretIcon open={open} />
      </button>
      {open && (
        <div className="absolute z-10 mt-1 min-w-full overflow-hidden rounded-md border border-gray-200 bg-white shadow-sm">
          {searchable && (
            <div className="border-b border-gray-200 p-1.5">
              <input
                ref={searchInputRef}
                type="text"
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value)
                }}
                placeholder="Search…"
                className="w-full rounded border border-gray-200 px-2 py-1 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
              />
            </div>
          )}
          <ul role="listbox" aria-multiselectable={props.multiple ? true : undefined} className="max-h-60 overflow-auto py-1">
            {visibleOptions.length === 0 && <li className="px-3 py-1.5 text-sm text-gray-400">No matches</li>}
            {visibleOptions.map((o) => {
              const selected = isSelected(o.value)
              return (
                <li key={o.value} role="option" aria-selected={selected}>
                  <button
                    type="button"
                    onClick={() => {
                      handleSelect(o.value)
                    }}
                    className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-gray-50 ${
                      selected ? 'font-medium text-gray-900' : 'text-gray-700'
                    }`}
                  >
                    {props.multiple && (
                      <span
                        className={`flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded border ${
                          selected ? 'border-gray-900 bg-gray-900 text-white' : 'border-gray-300'
                        }`}
                      >
                        {selected && <CheckIcon />}
                      </span>
                    )}
                    <span className="truncate">{o.label}</span>
                  </button>
                </li>
              )
            })}
          </ul>
        </div>
      )}
    </div>
  )
}

function CaretIcon({ open }: { open: boolean }) {
  return (
    <svg
      viewBox="0 0 20 20"
      fill="currentColor"
      className={`h-3.5 w-3.5 shrink-0 text-gray-400 transition-transform ${open ? 'rotate-180' : ''}`}
    >
      <path
        fillRule="evenodd"
        d="M5.23 7.21a.75.75 0 0 1 1.06.02L10 10.94l3.71-3.71a.75.75 0 1 1 1.06 1.06l-4.24 4.25a.75.75 0 0 1-1.06 0L5.21 8.29a.75.75 0 0 1 .02-1.08Z"
        clipRule="evenodd"
      />
    </svg>
  )
}

function CheckIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" className="h-2.5 w-2.5">
      <path d="M4 10l4 4 8-8" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
