import { useEffect, useRef, useState } from 'react'

// A hand-rolled listbox against Tailwind utility classes, no external
// UI library — same convention as ConfirmModal
// (plan/ai/frontend/frontend/step-07-confirm-modal.md). Replaces every
// native <select> in the app; see
// plan/ai/frontend/frontend/step-11-dropdown-component.md for why
// (multi-select support and consistent cross-browser styling neither
// gets natively).
export interface DropdownOption<V extends string = string> {
  value: V
  label: string
  disabled?: boolean
}

type DropdownProps<V extends string = string> =
  | {
      multiple?: false
      options: DropdownOption<V>[]
      value: V
      onChange: (value: V) => void
      placeholder?: string
      disabled?: boolean
    }
  | {
      multiple: true
      options: DropdownOption<V>[]
      value: V[]
      onChange: (value: V[]) => void
      placeholder?: string
      disabled?: boolean
    }

export function Dropdown<V extends string = string>(props: DropdownProps<V>) {
  const { options, disabled, placeholder } = props
  const [open, setOpen] = useState(false)
  const [highlighted, setHighlighted] = useState(0)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return

    const handlePointerDown = (e: MouseEvent) => {
      if (!containerRef.current?.contains(e.target as Node)) setOpen(false)
    }
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setOpen(false)
      } else if (e.key === 'ArrowDown') {
        e.preventDefault()
        setHighlighted((i) => Math.min(i + 1, options.length - 1))
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        setHighlighted((i) => Math.max(i - 1, 0))
      } else if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        selectOption(options[highlighted])
      }
    }
    document.addEventListener('mousedown', handlePointerDown)
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handlePointerDown)
      window.removeEventListener('keydown', handleKeyDown)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, highlighted, options])

  const selectOption = (opt: DropdownOption<V> | undefined) => {
    if (!opt || opt.disabled) return
    if (props.multiple) {
      const next = props.value.includes(opt.value) ? props.value.filter((v) => v !== opt.value) : [...props.value, opt.value]
      props.onChange(next)
      // Multi-select stays open — same convention as any checkbox-list listbox.
    } else {
      props.onChange(opt.value)
      setOpen(false)
    }
  }

  const buttonLabel = props.multiple
    ? summarize(options, props.value, placeholder)
    : (options.find((o) => o.value === props.value)?.label ?? placeholder ?? '')

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        disabled={disabled}
        onClick={() => {
          setOpen((v) => !v)
        }}
        aria-haspopup="listbox"
        aria-expanded={open}
        className="w-full rounded border border-gray-300 bg-white px-2 py-1.5 text-left text-sm disabled:opacity-50"
      >
        {buttonLabel}
      </button>
      {open && (
        <ul
          role="listbox"
          aria-multiselectable={props.multiple}
          className="absolute z-10 mt-1 max-h-60 w-full overflow-y-auto rounded border border-gray-200 bg-white shadow-lg"
        >
          {options.map((opt, i) => {
            const selected = props.multiple ? props.value.includes(opt.value) : opt.value === props.value
            return (
              // No onKeyDown here by design: focus never moves onto an
              // option <li> in this widget (options are activated by
              // mouse, or by Enter/Space via the document-level
              // keydown listener above while the trigger button holds
              // focus) — a per-item handler would never fire.
              // eslint-disable-next-line jsx-a11y/click-events-have-key-events
              <li
                key={opt.value}
                role="option"
                aria-selected={selected}
                onClick={() => {
                  selectOption(opt)
                }}
                onMouseEnter={() => {
                  setHighlighted(i)
                }}
                className={`flex cursor-pointer items-center px-2 py-1.5 text-sm ${i === highlighted ? 'bg-gray-100' : ''} ${
                  opt.disabled ? 'cursor-not-allowed opacity-50' : ''
                }`}
              >
                {props.multiple && <input type="checkbox" readOnly checked={selected} className="mr-2" />}
                {opt.label}
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

function summarize<V extends string>(options: DropdownOption<V>[], value: V[], placeholder?: string): string {
  if (value.length === 0) return placeholder ?? 'None selected'
  const labels = options.filter((o) => value.includes(o.value)).map((o) => o.label)
  if (labels.length <= 2) return labels.join(', ')
  return `${labels.length} selected`
}
