import type { PickField, StructuredHint, ViewElement } from './jobDetailPick'
import { FIELD_COLOR, FIELD_ORDER, isAmbiguousPick } from './fieldMarks'
import {
  FIELD_NAMES,
  hintsFor,
  isStructuredPick,
  pickFromHint,
  shortValue,
  type Pick,
  type Picks,
  type RuleDraft,
} from './jobDetailPickRules'
import type { PickCandidates } from './jobDetailPick'
import { buttonClass } from '../instructionStyles'

function stateOf(f: PickField, pick: Pick | null, rule: RuleDraft | null): { text: string; set: boolean } {
  if (f === 'description' && rule !== null) {
    const boxes = rule.parts.length
    return { text: `read with a rule (${String(boxes)} part${boxes === 1 ? '' : 's'})`, set: true }
  }
  if (!pick) return { text: f === 'description' ? 'not chosen yet' : 'not chosen', set: false }
  if (isStructuredPick(pick)) return { text: `${pick.value ? `${shortValue(pick.value, 50)} · ` : ''}from the page’s own data`, set: true }
  const reads = pick.attribute && pick.attribute !== 'text' ? ` · reads the ${pick.attribute} attribute` : ''
  return { text: `${pick.value ? shortValue(pick.value, 50) : 'kept from the stored instructions'}${reads}`, set: true }
}

// The five fields of the job in one row of chips (plan 249): what each is now, in the colour it has on the page, with the two actions that need no page: leave a field out and
// use what the page itself states. Everything else is said by clicking in the page, or by "Show the fields".
export function FieldStrip({
  page,
  view,
  picks,
  rule,
  onPick,
}: {
  page: PickCandidates
  view: ViewElement[]
  picks: Picks
  rule: RuleDraft | null
  onPick: (field: PickField, pick: Pick | null) => void
}) {
  return (
    <ul className="flex flex-wrap gap-2" data-testid="field-strip">
      {FIELD_ORDER.map((f) => {
        const pick = picks[f]
        const st = stateOf(f, pick, rule)
        const stated: StructuredHint | undefined = hintsFor(page, f).find((h) => h.source === 'jsonld') ?? hintsFor(page, f).at(0)
        const ambiguous = !(f === 'description' && rule !== null) && isAmbiguousPick(view, pick)
        return (
          <li
            key={f}
            data-testid={`field-chip-${f}`}
            className="flex min-w-0 max-w-full flex-col gap-1 rounded-xl border border-gray-200 bg-white px-3 py-2"
          >
            <span className="flex items-center gap-2">
              <span aria-hidden="true" className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ backgroundColor: FIELD_COLOR[f] }} />
              <span className="font-semibold text-gray-900">
                {FIELD_NAMES[f]}
                {f === 'description' && <span className="ml-0.5 text-red-700"> *</span>}
              </span>
            </span>
            <span className={`truncate text-xs ${st.set ? 'text-gray-800' : 'text-gray-500'}`}>{st.text}</span>
            {ambiguous && (
              <span className="text-[11px] text-amber-800" data-testid={`field-chip-${f}-ambiguous`}>
                Its address matches several places on the page; the first is read.
              </span>
            )}
            <span className="flex flex-wrap gap-1">
              {stated && f !== 'description' && !isStructuredPick(pick) && (
                <button
                  type="button"
                  className={buttonClass}
                  onClick={() => {
                    onPick(f, pickFromHint(stated))
                  }}
                >
                  Use the page’s own data
                </button>
              )}
              {pick && f !== 'description' && (
                <button
                  type="button"
                  className={buttonClass}
                  onClick={() => {
                    onPick(f, null)
                  }}
                >
                  Not on this page
                </button>
              )}
            </span>
          </li>
        )
      })}
    </ul>
  )
}
