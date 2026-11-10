// The look of the instruction modals' panels, in one place (plan/ai/tools/career/step-246-instruction-modal-two-columns-and-design.md): white page, neutral grays,
// one dark accent for the primary action and for a selection, green/amber/red only where they mean something. No blue surfaces.
const disabled = 'disabled:cursor-not-allowed disabled:opacity-50'

// A secondary action: an outline button.
export const buttonClass = `rounded-md border border-gray-300 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 ${disabled}`

// The main action of a block.
export const primaryClass = `rounded-md bg-gray-900 px-3 py-1.5 text-xs font-medium text-white hover:bg-gray-800 ${disabled}`

// A text input or a textarea.
export const inputClass =
  'rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none'

// A white card for what the person picks or reads (a list, a preview, a saved rule).
export const cardClass = 'rounded-xl border border-gray-200 bg-white p-4'

// A quiet card on a light gray, for notes.
export const mutedCardClass = 'rounded-xl border border-gray-200 bg-gray-50 p-4'
