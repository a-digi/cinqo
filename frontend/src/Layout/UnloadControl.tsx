import { useEffect, useId, useRef, useState } from 'react'
import { unloadDialogText, unloadLocalAgent, isInUse, agentName } from './agentUnload'
import type { AgentTone } from './resourceLevel'
import { refreshResources } from './useResources'

// The agent chip as a button with a small confirmation under it (plan 225). It is only rendered for a person who may unload, and only while a
// model is loading, working or idle. The chip's own look is passed in; this component adds the button, the dialog and the call.
export function UnloadControl({
  tone,
  name,
  label,
  className,
  children,
}: {
  tone: AgentTone
  name: string
  label: string
  className: string
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [inUse, setInUse] = useState(false) // the route said a conversation is using it: the dialog now asks for "anyway"
  const [error, setError] = useState('')
  const wrapRef = useRef<HTMLSpanElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const primaryRef = useRef<HTMLButtonElement>(null)
  const dialogRef = useRef<HTMLDivElement>(null)
  const dialogId = useId()
  const titleId = useId()

  const close = () => {
    setOpen(false)
    setInUse(false)
    setError('')
    buttonRef.current?.focus()
  }

  // outside click and Escape close it
  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) {
        setOpen(false)
        setInUse(false)
        setError('')
      }
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        close()
        return
      }
      if (e.key !== 'Tab' || !dialogRef.current) return
      // keep Tab inside the dialog
      const items = [...dialogRef.current.querySelectorAll<HTMLElement>('button:not([disabled])')]
      if (items.length === 0) return
      const first = items[0]
      const last = items[items.length - 1]
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  useEffect(() => {
    if (open) primaryRef.current?.focus()
  }, [open])

  const text = unloadDialogText(tone, agentName(name), inUse)

  const run = async () => {
    setBusy(true)
    setError('')
    try {
      await unloadLocalAgent(text.force)
      setOpen(false)
      setInUse(false)
      refreshResources()
    } catch (e) {
      if (isInUse(e)) {
        setInUse(true) // a conversation started between the click and the call: ask again, now for "anyway"
      } else {
        setError(e instanceof Error ? e.message : String(e))
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <span ref={wrapRef} className="relative inline-flex">
      <button
        ref={buttonRef}
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? dialogId : undefined}
        aria-label={label}
        title="Click to unload"
        onClick={() => {
          setOpen((v) => !v)
          setInUse(false)
          setError('')
        }}
        className={`${className} cursor-pointer hover:brightness-95 focus:outline-none focus-visible:ring-2 focus-visible:ring-teal-500`}
      >
        {children}
      </button>
      {open && (
        <div
          id={dialogId}
          ref={dialogRef}
          role="dialog"
          aria-labelledby={titleId}
          className="absolute right-0 top-full z-50 mt-2 w-72 rounded-xl border border-slate-200 bg-white p-3 text-left shadow-xl"
        >
          <p id={titleId} className="text-sm font-semibold text-slate-900">
            {text.title}
          </p>
          <p className="mt-1 text-xs text-slate-600">{text.body}</p>
          {error !== '' && (
            <p role="alert" className="mt-2 text-xs text-red-700">
              {error}
            </p>
          )}
          <div className="mt-3 flex justify-end gap-2">
            <button type="button" onClick={close} className="rounded-md px-3 py-1 text-xs font-medium text-slate-600 hover:bg-slate-100">
              Cancel
            </button>
            <button
              ref={primaryRef}
              type="button"
              disabled={busy}
              onClick={() => void run()}
              className="rounded-md bg-slate-800 px-3 py-1 text-xs font-semibold text-white hover:bg-slate-700 disabled:opacity-60"
            >
              {busy ? 'Unloading…' : text.action}
            </button>
          </div>
        </div>
      )}
    </span>
  )
}
