import { proposalRowTone, type ProposalItem, type ProposalRowTone } from './parseProposal'

// One shared row renderer, reused across all 5 areas — every area's
// own proposal items share the uniform {action, possibleDuplicateOf}
// shape (step 5's own contract), so one renderer + one tone function
// (parseProposal.ts's own proposalRowTone) covers Profile/Persona/
// Personal Details/Skills/Experience alike instead of five bespoke
// ones. See
// plan/ai/tools/career/import-cv/step-06-proposal-parsing-and-review-ui.md.
const toneClasses: Record<ProposalRowTone, string> = {
  gray: 'border-gray-200 bg-white',
  amber: 'border-amber-200 bg-amber-50',
  red: 'border-red-200 bg-red-50',
}

export function ProposalRow({
  item,
  label,
  resolvedDuplicateLabel,
  checked,
  onToggle,
  done = false,
  errorMessage,
}: {
  item: ProposalItem
  label: string
  // Human-readable resolution of item.possibleDuplicateOf — an id for
  // persona/experience (resolveDuplicateLabels.ts), or the raw string
  // itself for a skill (already readable, no id exists to resolve).
  resolvedDuplicateLabel?: string
  checked: boolean
  onToggle: () => void
  // true once this exact item has been successfully inserted in a
  // previous "Insert" click (step 7) — locks the checkbox and shows a
  // distinct "Inserted" state instead of the normal tone coloring, so
  // a second click can never re-create the same row. Persists across
  // retries (tracked by the page's own insertedKeys, not reset by a
  // failed retry of other items).
  done?: boolean
  // Set when the most recent insert attempt for this exact item
  // failed — a distinct indicator from the "possible duplicate" red,
  // since they mean different things (one is an AI judgment made
  // before insert; this is a real error from the insert call itself).
  errorMessage?: string
}) {
  const tone = proposalRowTone(item)
  return (
    <label
      className={`flex items-start gap-2 rounded-lg border px-3 py-2 text-sm ${done ? 'cursor-default border-green-200 bg-green-50' : `cursor-pointer ${toneClasses[tone]}`}`}
    >
      <input type="checkbox" checked={done || checked} disabled={done} onChange={onToggle} className="mt-0.5" />
      <span>
        <span className="block text-gray-900">{label}</span>
        {item.possibleDuplicateOf !== null && !done && (
          <span className="block text-xs text-red-700">Possible duplicate of: {resolvedDuplicateLabel ?? item.possibleDuplicateOf}</span>
        )}
        {done && <span className="block text-xs text-green-700">Inserted</span>}
        {errorMessage && !done && <span className="block text-xs text-red-700">Failed: {errorMessage}</span>}
      </span>
    </label>
  )
}
