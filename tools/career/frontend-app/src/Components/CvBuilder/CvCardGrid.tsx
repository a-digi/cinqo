import { mediaDownloadUrl, type CvDocument, type CvTemplate, type Persona } from '../../api'
import { CV_BUILDER_PATH } from '../../paths'
import { CvCard } from './CvCard'
import { CARD_MIN_REM, templateLabel } from './cvCardRules'

// The management actions of a CV card: whichever a page passes are drawn (rename, comments and delete in the
// card's menu, the attach picker and the detach buttons on the persona chips). A read-only page passes none.
export interface CvCardManagement {
  onRename?: (doc: CvDocument) => void
  onComments?: (doc: CvDocument) => void
  onDelete?: (doc: CvDocument) => void
  // Every persona, to offer the ones a CV can still be attached to.
  allPersonas?: Persona[]
  onAttach?: (doc: CvDocument, personaId: string) => void
  onDetach?: (doc: CvDocument, personaId: string) => void
}

// The grid the CV cards sit in: the same columns and gaps everywhere, so a CV card is the same size on every
// page. A uniquely-named class, as the Media page does: an installed tool's globally-injected Tailwind
// stylesheet can redeclare a bare `.grid-cols-*` and collapse a plain Tailwind grid.
function Frame({ children, className = '', label }: { children: React.ReactNode; className?: string; label?: string }) {
  return (
    <>
      <style>{`
        .cv-card-grid { display: grid; gap: 1.5rem 1rem; grid-template-columns: repeat(auto-fill, minmax(${CARD_MIN_REM}rem, 1fr)); }
      `}</style>
      <div className={`cv-card-grid ${className}`.trim()} aria-label={label}>
        {children}
      </div>
    </>
  )
}

// Career's one way to show CVs as preview cards (the CV Documents page, the profile view, the persona
// view): the preview, the title, the template, the personas, and Preview/Edit that open the PDF and the
// builder. aiChats maps a CV's id to the chat of the AI run that created it (chatsByCv), for the "Created
// with AI" mark that opens that chat; highlightedId rings the CV an AI run just made.
export function CvCardGrid({
  docs,
  templates,
  aiChats,
  highlightedId,
  management,
  className,
}: {
  docs: CvDocument[]
  templates: CvTemplate[]
  aiChats?: ReadonlyMap<string, string>
  highlightedId?: string | null
  management?: CvCardManagement
  className?: string
}) {
  return (
    <Frame className={className}>
      {docs.map((doc) => {
        const chat = aiChats?.get(doc.id)
        return (
          <CvCard
            key={doc.id}
            doc={doc}
            templateName={templateLabel(templates, doc.templateId)}
            onPreview={() => {
              window.open(mediaDownloadUrl(doc.mediaFileId), '_blank', 'noopener,noreferrer')
            }}
            onEdit={() => {
              window.__cinqoToolBridge.navigate(`${CV_BUILDER_PATH}?id=${encodeURIComponent(doc.id)}`)
            }}
            onRename={management?.onRename && (() => management.onRename?.(doc))}
            onComments={management?.onComments && (() => management.onComments?.(doc))}
            onDelete={management?.onDelete && (() => management.onDelete?.(doc))}
            allPersonas={management?.allPersonas}
            onAttach={management?.onAttach && ((personaId) => management.onAttach?.(doc, personaId))}
            onDetach={management?.onDetach && ((personaId) => management.onDetach?.(doc, personaId))}
            onShowAiChat={
              chat === undefined
                ? undefined
                : () => {
                    window.__cinqoToolBridge.openConversation(chat)
                  }
            }
            highlighted={doc.id === highlightedId}
          />
        )
      })}
    </Frame>
  )
}

// Placeholder tiles while the CVs are being read, in the same grid.
export function CvCardGridSkeleton() {
  return (
    <Frame label="Loading CVs">
      {[0, 1, 2, 3].map((i) => (
        <div key={i} className="animate-pulse">
          <div className="aspect-[3/4] rounded-xl bg-gray-100" />
          <div className="mt-2 h-3 w-3/4 rounded bg-gray-100" />
          <div className="mt-2 h-3 w-1/2 rounded bg-gray-100" />
        </div>
      ))}
    </Frame>
  )
}
