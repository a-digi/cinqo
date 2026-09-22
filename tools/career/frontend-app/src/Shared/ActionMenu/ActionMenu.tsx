import { useEffect, useRef, useState, type ReactNode } from 'react'
import { ChevronDownIcon, DotsVerticalIcon } from '../Icons/icons'

// One row in the menu — either a click action (onClick), a plain link
// (href, e.g. "open the original posting" or "download this file"),
// or a group with its own nested items (children). At most one of
// onClick/href is expected on any given item; a LEAF item (no
// children) must set one of the two so it always does something when
// clicked — a PARENT item (children present) may set neither, since
// its own row can be purely a toggle for the children below it.
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
  // children (step XX) turns this row into an expandable group — a
  // caret-down toggle appears, and clicking it reveals this item's own
  // nested actions indented below it, instead of firing anything
  // itself. Recursive (a child may itself declare its own children),
  // each level indented 10px further than its own parent — though the
  // one real use case this exists for today (a job's own "CV" entry
  // expanding into "Download CV" / "Re-generate with AI" once a CV
  // already exists) only ever goes one level deep. See
  // plan/ai/tools/career/step-XX-jobs-page-cv-regenerate.md.
  children?: ActionMenuItem[]
}

export interface ActionMenuProps {
  items: ActionMenuItem[]
  // Accessible name for the trigger button — a screen reader has no
  // other way to know what "⋮" opens.
  triggerLabel?: string
}

function variantClassName(variant: ActionMenuItem['variant']): string {
  if (variant === 'danger') return 'text-red-700 hover:bg-red-50'
  if (variant === 'success') return 'text-green-700 hover:bg-green-50'
  return 'text-gray-700 hover:bg-gray-50'
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
  // expandedKey — at most one parent item's own children are shown at
  // a time (one level of nesting, no reason to allow more than one
  // group open together in a menu this small). Reset whenever the
  // menu itself closes, below, so reopening it never starts
  // pre-expanded from a previous open.
  const [expandedKey, setExpandedKey] = useState<string | null>(null)
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

  useEffect(() => {
    if (!open) setExpandedKey(null)
  }, [open])

  // depth — how many ancestor `children` levels this item is nested
  // under (0 = a top-level item). Each level shifts its own row 10px
  // further right than its own parent, so nesting reads visually as
  // nesting no matter how deep it goes.
  function renderLeafItem(item: ActionMenuItem, depth = 0) {
    const content = (
      <>
        {item.icon && <span className="shrink-0">{item.icon}</span>}
        <span className="truncate">{item.label}</span>
      </>
    )
    const className = `flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${variantClassName(item.variant)}`
    const style = depth > 0 ? { marginLeft: depth * 10 } : undefined

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
          style={style}
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
        style={style}
      >
        {content}
      </button>
    )
  }

  function renderItem(item: ActionMenuItem, depth = 0) {
    if (!item.children || item.children.length === 0) {
      return renderLeafItem(item, depth)
    }

    const expanded = expandedKey === item.key
    const hasOwnAction = Boolean(item.onClick ?? item.href)
    const rowContent = (
      <>
        {item.icon && <span className="shrink-0">{item.icon}</span>}
        <span className="truncate">{item.label}</span>
      </>
    )
    const rowClassName = `flex flex-1 items-center gap-2 px-3 py-2 text-left text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${variantClassName(item.variant)}`
    const rowStyle = depth > 0 ? { marginLeft: depth * 10 } : undefined

    return (
      <div key={item.key}>
        <div className="flex items-center">
          {hasOwnAction ? (
            item.href ? (
              <a
                role="menuitem"
                href={item.href}
                target={item.target}
                rel={item.rel}
                title={item.title}
                onClick={() => {
                  setOpen(false)
                }}
                className={rowClassName}
                style={rowStyle}
              >
                {rowContent}
              </a>
            ) : (
              <button
                type="button"
                role="menuitem"
                disabled={item.disabled}
                title={item.title}
                onClick={() => {
                  setOpen(false)
                  item.onClick?.()
                }}
                className={rowClassName}
                style={rowStyle}
              >
                {rowContent}
              </button>
            )
          ) : (
            <button
              type="button"
              title={item.title}
              onClick={() => {
                setExpandedKey(expanded ? null : item.key)
              }}
              className={rowClassName}
              style={rowStyle}
            >
              {rowContent}
            </button>
          )}
          <button
            type="button"
            aria-label={expanded ? `Collapse ${item.label}` : `Expand ${item.label}`}
            aria-expanded={expanded}
            onClick={() => {
              setExpandedKey(expanded ? null : item.key)
            }}
            className="shrink-0 px-2.5 py-2 text-gray-400 transition-colors hover:text-gray-700"
          >
            <span className={`inline-block transition-transform duration-150 ${expanded ? 'rotate-180' : ''}`}>
              <ChevronDownIcon />
            </span>
          </button>
        </div>
        {expanded && (
          <div role="group" className="border-t border-gray-100 bg-gray-50 py-1">
            {item.children.map((child) => renderItem(child, depth + 1))}
          </div>
        )}
      </div>
    )
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
          {items.map((item) => renderItem(item))}
        </div>
      )}
    </div>
  )
}
