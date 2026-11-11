import { useState } from 'react'

import { PlusIcon, XIcon } from '../../Shared/Icons/icons'
import { Field } from '../Field/Field'
import { canAdd, canSave, type SectionSpec } from './cvSectionRules'

// One section of the wizard step "Education & more" (plan 251): the entries of the CV as lines, each one removable (the x) and editable (click it), and "Add" for a
// new one. The entries came from the profile and are the CV's own copy: nothing done here is ever written back to the profile.
export function SectionList<T>({ spec, items, onChange }: { spec: SectionSpec<T>; items: T[]; onChange: (next: T[]) => void }) {
  // null: nothing is being edited; an index: that entry; items.length: a new one
  const [editing, setEditing] = useState<number | null>(null)
  const [form, setForm] = useState<T>(spec.blank)
  const [tried, setTried] = useState(false)

  const close = () => {
    setEditing(null)
    setForm(spec.blank)
    setTried(false)
  }
  const save = () => {
    if (editing === null) return
    if (!canSave(spec, form)) {
      setTried(true)
      return
    }
    const next = [...items]
    if (editing < next.length) next[editing] = form
    else next.push(form)
    onChange(next)
    close()
  }
  const remove = (index: number) => {
    onChange(items.filter((_, i) => i !== index))
    if (editing !== null) close()
  }
  const requiredLabel = spec.fields.find((f) => f.key === spec.required)?.label ?? spec.required

  const formView = (
    <div className="space-y-2 rounded-md border border-gray-200 bg-white p-2" data-testid={`cv-section-form-${spec.key}`}>
      {spec.fields.map((f) =>
        f.multiline ? (
          <label key={f.key} className="block text-xs font-medium text-gray-500">
            {f.label}
            <textarea
              value={String(form[f.key])}
              onChange={(e) => {
                setForm({ ...form, [f.key]: e.target.value })
              }}
              rows={2}
              className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
            />
          </label>
        ) : (
          <Field
            key={f.key}
            label={f.label}
            value={String(form[f.key])}
            onChange={(v) => {
              setForm({ ...form, [f.key]: v })
            }}
          />
        ),
      )}
      {tried && !canSave(spec, form) && (
        <p role="alert" className="text-xs text-red-700">
          {requiredLabel} is required.
        </p>
      )}
      <div className="flex gap-2">
        <button
          type="button"
          onClick={save}
          className="rounded-md bg-gray-900 px-2.5 py-1 text-xs font-medium text-white hover:bg-gray-800"
        >
          Save
        </button>
        <button
          type="button"
          onClick={close}
          className="rounded-md border border-gray-300 px-2.5 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50"
        >
          Cancel
        </button>
      </div>
    </div>
  )

  return (
    <section aria-label={spec.title} className="rounded-xl border border-gray-200 bg-white p-3" data-testid={`cv-section-${spec.key}`}>
      <h3 className="mb-2 flex items-baseline justify-between text-xs font-semibold uppercase tracking-wide text-gray-500">
        <span>{spec.title}</span>
        <span className="font-normal normal-case text-gray-400">
          {items.length} of {spec.max}
        </span>
      </h3>
      <div className="space-y-2">
        {items.length === 0 && editing === null && <p className="text-xs text-gray-400">{spec.emptyText}</p>}
        {items.map((entry, i) => {
          if (editing === i) return <div key={i}>{formView}</div>
          const s = spec.summary(entry)
          return (
            <div key={i} className="flex items-start justify-between gap-2 rounded-md border border-gray-200 p-2">
              <button
                type="button"
                onClick={() => {
                  setEditing(i)
                  setForm(entry)
                  setTried(false)
                }}
                className="min-w-0 flex-1 text-left"
                aria-label={`Edit ${s.main}`}
              >
                <p className="truncate text-sm font-medium text-gray-900">{s.main}</p>
                {s.sub && <p className="truncate text-xs text-gray-500">{s.sub}</p>}
              </button>
              <button
                type="button"
                onClick={() => {
                  remove(i)
                }}
                className="shrink-0 text-gray-400 hover:text-red-600"
                aria-label={`Remove ${s.main} from this CV`}
              >
                <XIcon />
              </button>
            </div>
          )
        })}
        {editing === items.length && formView}
      </div>
      {editing === null &&
        (canAdd(spec, items.length) ? (
          <button
            type="button"
            onClick={() => {
              setEditing(items.length)
              setForm(spec.blank)
              setTried(false)
            }}
            className="mt-2 flex items-center gap-1 text-xs font-medium text-gray-700 hover:text-gray-900"
          >
            <PlusIcon /> Add {spec.noun}
          </button>
        ) : (
          <p className="mt-2 text-xs text-gray-500">The most a CV can hold is {spec.max}.</p>
        ))}
    </section>
  )
}
