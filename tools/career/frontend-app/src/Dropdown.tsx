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
    }
  | {
      multiple: true
      options: DropdownOption[]
      value: string[]
      onChange: (value: string[]) => void
      label?: string
      placeholder?: string
    }

// Shared single/multiple-select dropdown — a real popover listbox,
// not a native <select>, so multiple mode gets real checkbox-style
// UX (toggle a row, popover stays open for the next pick) instead of
// native <select multiple>'s own ctrl/cmd-click behavior with no
// visible "currently selected" summary. A discriminated union on
// `multiple` gives every call site a correctly-typed `value`/
// `onChange` (string vs. string[]) with no cast needed. See
// plan/ai/tools/career/step-13-dropdown-component.md.
export function Dropdown(props: DropdownProps) {
  const { options, label, placeholder = 'Select…' } = props
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
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
        onClick={() => setOpen((prev) => !prev)}
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
        <ul
          role="listbox"
          aria-multiselectable={props.multiple || undefined}
          className="absolute z-10 mt-1 max-h-60 min-w-full overflow-auto rounded-md border border-gray-200 bg-white py-1 shadow-sm"
        >
          {options.map((o) => {
            const selected = isSelected(o.value)
            return (
              <li key={o.value} role="option" aria-selected={selected}>
                <button
                  type="button"
                  onClick={() => handleSelect(o.value)}
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
