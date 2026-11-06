import { useId } from 'react'
import { KEEP_LOGIN_LABEL, keepLoginExplanation } from '../loginSession'

// KeepLoginOption (plan 195, step 2): the opt-in under "I am logged in". Off unless the person ticks it.
export function KeepLoginOption({ checked, onChange, disabled }: { checked: boolean; onChange: (keep: boolean) => void; disabled?: boolean }) {
  const id = useId()
  return (
    <div className="mt-1.5 flex items-start gap-1.5 text-xs text-sky-900">
      <input
        id={id}
        type="checkbox"
        className="mt-0.5"
        checked={checked}
        disabled={disabled}
        onChange={(e) => {
          onChange(e.target.checked)
        }}
      />
      <label htmlFor={id}>
        <span className="font-medium">{KEEP_LOGIN_LABEL}</span>
        <span className="block text-sky-800">{keepLoginExplanation()}</span>
      </label>
    </div>
  )
}
