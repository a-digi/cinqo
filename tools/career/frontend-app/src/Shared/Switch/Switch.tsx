interface SwitchProps {
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  // Optional trailing text, rendered inside the same <label> as the
  // switch itself so clicking the text also toggles it (a <button> is
  // a labelable element per the HTML spec, so this "just works" the
  // same way a native checkbox + label pair does). Omit for a
  // label-less switch (the caller supplies its own surrounding text).
  label?: string
  // 'sm' matches the smaller per-portal-link toggle
  // (Crawler/CrawlPanel/CrawlPanel.tsx); 'md' (default) matches the
  // larger page-wide toggle (Crawler/AutoDiscoveryPanel/AutoDiscoveryPanel.tsx).
  // Both sizes are real, currently-used call sites, not a hypothetical
  // future one.
  size?: 'sm' | 'md'
}

// Shared on/off toggle — stateless, controlled, no fetch calls, same
// "dumb, reusable" convention Shared/Modal/Modal.tsx and
// Shared/InfoBox/InfoBox.tsx already established. Introduced to
// replace the two native `<input type="checkbox">` auto-discovery
// toggles (global + per-link) with one shared, visually consistent
// control.
export function Switch({ checked, onChange, disabled = false, label, size = 'md' }: SwitchProps) {
  const track = size === 'sm' ? 'h-4 w-7' : 'h-5 w-9'
  const knob = size === 'sm' ? 'h-3 w-3' : 'h-4 w-4'
  const knobOn = size === 'sm' ? 'translate-x-3' : 'translate-x-4'
  const textSize = size === 'sm' ? 'text-xs' : 'text-sm'

  const control = (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label ? undefined : 'Toggle'}
      disabled={disabled}
      onClick={() => {
        onChange(!checked)
      }}
      className={`relative inline-flex shrink-0 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-gray-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:opacity-50 ${track} ${
        checked ? 'bg-gray-900' : 'bg-gray-200'
      }`}
    >
      <span
        className={`inline-block transform rounded-full bg-white shadow transition-transform ${knob} ${
          checked ? knobOn : 'translate-x-0.5'
        }`}
      />
    </button>
  )

  if (!label) return control

  return (
    <label className={`flex items-center gap-2 ${textSize} text-gray-700 ${disabled ? 'cursor-not-allowed' : 'cursor-pointer'}`}>
      {control}
      {label}
    </label>
  )
}
