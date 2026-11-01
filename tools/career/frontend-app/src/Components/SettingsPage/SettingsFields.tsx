// The two form fields the settings pages share (step 150: moved here from the Job cleanup page, which now imports them): a number
// box that keeps what was typed and shows its error, and a tick box with a label and a hint.

export function NumberField(props: { label: string; value: string; error?: string; unit: string; onChange: (v: string) => void }) {
  return (
    <label className="block text-sm">
      <span className="text-gray-700">{props.label}</span>
      <span className="mt-1 flex items-center gap-2">
        <input
          type="text"
          inputMode="numeric"
          value={props.value}
          onChange={(e) => {
            props.onChange(e.target.value)
          }}
          aria-invalid={props.error !== undefined}
          className={`w-24 rounded-lg border px-3 py-1.5 text-sm ${props.error ? 'border-red-400' : 'border-gray-300'}`}
        />
        <span className="text-gray-500">{props.unit}</span>
      </span>
      {props.error && <span className="mt-1 block text-xs text-red-700">{props.error}</span>}
    </label>
  )
}

export function Check(props: { label: string; hint?: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex items-start gap-2 text-sm">
      <input
        type="checkbox"
        checked={props.checked}
        onChange={(e) => {
          props.onChange(e.target.checked)
        }}
        className="mt-1"
      />
      <span>
        <span className="font-medium text-gray-900">{props.label}</span>
        {props.hint && <span className="block text-xs text-gray-500">{props.hint}</span>}
      </span>
    </label>
  )
}
