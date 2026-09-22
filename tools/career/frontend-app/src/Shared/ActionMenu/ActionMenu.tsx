import { useEffect, useRef, useState, type ReactNode } from 'react'
import { DotsVerticalIcon } from '../Icons/icons'

// One row in the menu — either a click action (onClick) or a plain
// link (href, e.g. "open the original posting" or "download this
// file"); exactly one of the two is expected per item, matching how
// each of this menu's own real call sites already needed one or the
// other, never both.
export interface ActionMenuItem {
  key: string
  label: string
  icon?: ReactNode
  onClick?: () => void
  href?: string
  target?: string
  rel?: string
  disabled?: boolean
  // title doubles as the native tooltip AND the reason shown when
  // disabled (e.g. "Create a profile first") — same single-purpose
  // convention every existing icon button in this tool already used
  // before being folded into this menu.
  title?: string
  // 'danger' (destructive, e.g. delete) / 'success' (a positive,
  // ready-to-use outcome, e.g. "CV ready — download") get their own
  // accent color; 'default' (omitted) is plain gray. Mirrors the
  // color-coding convention already established on JobsPage.tsx's own
  // per-row icons (green for "CV ready", red for "failed"/delete)
  // before this component existed.
  variant?: 'default' | 'danger' | 'success'
}

export interface ActionMenuProps {
  items: ActionMenuItem[]
  // Accessible name for the trigger button — a screen reader has no
  // other way to know what "⋮" opens.
  triggerLabel?: string
}

// ActionMenu — a shared "three dots" kebab menu: a single trigger
// button that opens a dropdown listing every row action, replacing a
// wide row of individual icon buttons (JobsPage.tsx's own original
// Actions column) with one consistent, space-efficient control. Built
// once here so any other page in this tool needing the same
// "one button, several row actions" pattern reuses it rather than
// re-implementing its own open/close/click-outside handling. Same
// click-outside + Escape-to-close convention already established by
// Components/Dropdown/Dropdown.tsx — this is a second, independent
// implementation (not a shared base) since the two are visually and
// structurally different enough (a listbox of selectable options vs.
// a menu of one-shot actions) that sharing code would mean threading
// divergent behavior through one component. See
// plan/ai/tools/career/step-XX-jobs-page-action-menu.md.
export function ActionMenu({ items, triggerLabel = 'Actions' }: ActionMenuProps) {
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

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

  function variantClassName(variant: ActionMenuItem['variant']): string {
    if (variant === 'danger') return 'text-red-700 hover:bg-red-50'
    if (variant === 'success') return 'text-green-700 hover:bg-green-50'
    return 'text-gray-700 hover:bg-gray-50'
  }

  return (
    <div ref={containerRef} className="relative inline-block text-left">
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={triggerLabel}
        onClick={() => {
          setOpen((prev) => !prev)
        }}
        className="rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900"
      >
        <DotsVerticalIcon />
      </button>
      {open && (
        <div
          role="menu"
          className="absolute right-0 z-10 mt-1 min-w-48 overflow-hidden rounded-md border border-gray-200 bg-white py-1 shadow-lg"
        >
          {items.map((item) => {
            const content = (
              <>
                {item.icon && <span className="shrink-0">{item.icon}</span>}
                <span className="truncate">{item.label}</span>
              </>
            )
            const className = `flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${variantClassName(item.variant)}`

            if (item.href) {
              return (
                <a
                  key={item.key}
                  role="menuitem"
                  href={item.href}
                  target={item.target}
                  rel={item.rel}
                  title={item.title}
                  onClick={() => {
                    setOpen(false)
                  }}
                  className={className}
                >
                  {content}
                </a>
              )
            }
            return (
              <button
                key={item.key}
                type="button"
                role="menuitem"
                disabled={item.disabled}
                title={item.title}
                onClick={() => {
                  setOpen(false)
                  item.onClick?.()
                }}
                className={className}
              >
                {content}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
