import { Dropdown } from '../../Components/Dropdown/Dropdown'
import type { Platform } from '../../Cinqo/Platform/platformRepository'

// The shared AI-platform/model choice CrawlPanel's own "Crawl with
// AI"/"Generate with AI" both use — picked once per page, not per
// link, since switching platforms mid-session for one link only would
// be a strange UX with no real use case. Hidden entirely when there's
// only one configured platform (nothing to choose). Extracted out of
// PortalsPage.tsx, step 42. See
// plan/ai/tools/career/step-21-portals-frontend.md and
// plan/ai/tools/career/step-42-crawler-folder-reorganization.md.
export function CrawlPlatformPicker({
  platforms,
  selectedPlatformId,
  selectedModel,
  onSelectPlatform,
  onSelectModel,
}: {
  platforms: Platform[]
  selectedPlatformId: string | null
  selectedModel: string | null
  onSelectPlatform: (id: string) => void
  onSelectModel: (model: string) => void
}) {
  if (platforms.length <= 1) return null
  const selectedPlatform = platforms.find((p) => p.id === selectedPlatformId) ?? null

  return (
    <div className="mb-5 flex flex-wrap items-end gap-3 rounded-md border border-gray-200 bg-gray-50 p-3">
      <div className="min-w-[180px]">
        <label className="mb-1 block text-xs font-medium text-gray-500">Crawl using</label>
        <Dropdown
          options={platforms.map((p) => ({ value: p.id, label: p.name }))}
          value={selectedPlatformId}
          onChange={onSelectPlatform}
        />
      </div>
      {selectedPlatform && selectedPlatform.models.length > 0 && (
        <div className="min-w-[180px]">
          <label className="mb-1 block text-xs font-medium text-gray-500">Model</label>
          <Dropdown
            options={selectedPlatform.models.map((m) => ({ value: m, label: m }))}
            value={selectedModel}
            onChange={onSelectModel}
          />
        </div>
      )}
    </div>
  )
}
