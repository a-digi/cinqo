import { useEffect, useState } from 'react'
import { ApiError } from '../../api/client'
import { fetchWidgets, saveLayout, type WidgetDefinition } from '../../api/dashboard'
import { useSnackBar } from '../../Shared/Components/SnackBar/SnackBarContext'
import { LoadingSpinner } from '../../Shared/Components/Loading/LoadingSpinner'
import { CustomizePanel } from './CustomizePanel'
import { moveItem, sameLayout, toPayload, toggleHidden, visibleItems } from './layoutRules'
import { WidgetFrame } from './WidgetFrame'

// A uniquely-named grid class, for the same reason as the card grids: an
// installed tool's globally-injected Tailwind stylesheet can redeclare a bare
// `.grid-cols-*` and collapse a plain Tailwind grid. A widget's width spans that
// many columns, capped by how many columns the screen has (1, 2 or 4).
const gridCss = `
  .home-widget-grid { display: grid; gap: 1rem; grid-template-columns: 1fr; }
  .home-widget { min-width: 0; }
  @media (min-width: 640px) {
    .home-widget-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); grid-auto-rows: minmax(9rem, auto); }
    .home-widget[data-w="2"], .home-widget[data-w="3"], .home-widget[data-w="4"] { grid-column: span 2; }
    .home-widget[data-h="2"] { grid-row: span 2; }
    .home-widget[data-h="3"] { grid-row: span 3; }
  }
  @media (min-width: 1024px) {
    .home-widget-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); }
    .home-widget[data-w="2"] { grid-column: span 2; }
    .home-widget[data-w="3"] { grid-column: span 3; }
    .home-widget[data-w="4"] { grid-column: span 4; }
  }
`

// The signed-in landing page: a grid of widgets, each loading and refreshing its
// own data. See plan/ai/dashboard/home-dashboard.md.
export function HomePage() {
  const [widgets, setWidgets] = useState<WidgetDefinition[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  // The Customize view works on a draft; nothing is saved until Save.
  const [draft, setDraft] = useState<WidgetDefinition[] | null>(null)
  const [saving, setSaving] = useState(false)
  const { successMessage, errorMessage } = useSnackBar()

  useEffect(() => {
    let cancelled = false
    fetchWidgets()
      .then((list) => {
        if (!cancelled) setWidgets(list)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load the dashboard.')
      })
    return () => {
      cancelled = true
    }
  }, [])

  const handleSave = async () => {
    if (!draft) return
    setSaving(true)
    try {
      await saveLayout(toPayload(draft))
      setWidgets(draft)
      setDraft(null)
      successMessage('Dashboard saved.')
    } catch (err) {
      errorMessage(err instanceof ApiError ? err.message : 'Failed to save the dashboard.')
    } finally {
      setSaving(false)
    }
  }

  if (error) return <p className="p-6 text-sm text-red-600">{error}</p>
  if (!widgets) {
    return (
      <div className="flex min-h-64 items-center justify-center">
        <LoadingSpinner label="Loading dashboard…" />
      </div>
    )
  }

  return (
    <div className="max-w-7xl space-y-6 p-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Home</h1>
          <p className="mt-1 text-sm text-gray-500">
            {draft ? 'Choose which widgets to show and in what order.' : 'The most important information, at a glance.'}
          </p>
        </div>
        {widgets.length > 0 && !draft && (
          <button
            type="button"
            onClick={() => {
              setDraft(widgets)
            }}
            className="shrink-0 rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
          >
            Customize
          </button>
        )}
        {draft && (
          <div className="flex shrink-0 gap-2">
            <button
              type="button"
              disabled={saving}
              onClick={() => {
                setDraft(null)
              }}
              className="rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="button"
              disabled={saving || sameLayout(draft, widgets)}
              onClick={() => {
                void handleSave()
              }}
              className="rounded-md bg-gray-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
            >
              {saving ? 'Saving…' : 'Save'}
            </button>
          </div>
        )}
      </div>

      {draft ? (
        <CustomizePanel
          widgets={draft}
          disabled={saving}
          onMove={(id, direction) => {
            setDraft((d) => (d ? moveItem(d, id, direction) : d))
          }}
          onToggle={(id) => {
            setDraft((d) => (d ? toggleHidden(d, id) : d))
          }}
        />
      ) : widgets.length === 0 ? (
        <p className="py-6 text-center text-sm text-gray-400">No widgets to show yet.</p>
      ) : visibleItems(widgets).length === 0 ? (
        <p className="py-6 text-center text-sm text-gray-400">All widgets are hidden. Use Customize to show some.</p>
      ) : (
        <>
          <style>{gridCss}</style>
          <div className="home-widget-grid">
            {visibleItems(widgets).map((widget, index) => (
              <div key={widget.id} className="home-widget" data-w={widget.size.w} data-h={widget.size.h}>
                <WidgetFrame widget={widget} animationDelayMs={Math.min(index, 12) * 200} />
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
