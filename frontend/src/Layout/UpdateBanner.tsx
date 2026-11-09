import { useNewVersion } from './useNewVersion'

// A slim bar under the top bar that appears by itself when the app was rebuilt while this window stayed open (plan 227). One click reloads.
export function UpdateBanner() {
  const available = useNewVersion()
  if (!available) return null
  return (
    <div
      role="status"
      className="flex shrink-0 items-center justify-center gap-3 border-b border-sky-200 bg-sky-50 px-4 py-1.5 text-xs text-sky-900"
    >
      <span>A new version of Cinqo is available.</span>
      <button
        type="button"
        onClick={() => {
          window.location.reload()
        }}
        className="rounded-md bg-sky-700 px-2.5 py-1 font-semibold text-white hover:bg-sky-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-500"
      >
        Reload
      </button>
    </div>
  )
}
