// Shared labeled-input control — used by ProfilePage and
// ExperiencePage. Local to this tool's own frontend-app (not imported
// from the main app's own component tree), same self-containment rule
// every other file here already follows.
export function Field({
  label,
  value,
  onChange,
  type = 'text',
  id,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  type?: string
  // Lets a page focus the input (the Summary & Goals stepper does).
  id?: string
}) {
  return (
    <label className="block text-xs font-medium text-gray-500">
      {label}
      <input
        id={id}
        type={type}
        value={value}
        onChange={(e) => {
          onChange(e.target.value)
        }}
        className="mt-1 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
      />
    </label>
  )
}
