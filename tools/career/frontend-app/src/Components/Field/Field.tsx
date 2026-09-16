// Shared labeled-input control — used by ProfilePage and
// ExperiencePage. Local to this tool's own frontend-app (not imported
// from the main app's own component tree), same self-containment rule
// every other file here already follows.
export function Field({
  label,
  value,
  onChange,
  type = 'text',
}: {
  label: string
  value: string
  onChange: (v: string) => void
  type?: string
}) {
  return (
    <label className="block text-xs font-medium text-gray-500">
      {label}
      <input
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
