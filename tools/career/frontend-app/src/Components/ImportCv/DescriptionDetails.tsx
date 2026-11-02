import { descriptionSummary } from './descriptionPreviewRules'

// A collapsed preview of the text of one proposed job (plan 177, step 3): it opens to the whole description as plain text with its line
// breaks, in a box that scrolls when the text is long. It sits outside the row's label, so opening it never ticks or unticks the row.
export function DescriptionDetails({ description }: { description: string }) {
  const summary = descriptionSummary(description)
  if (description.trim() === '') return <p className="ml-7 mt-1 text-xs text-gray-400">{summary}</p>
  return (
    <details className="ml-7 mt-1 text-xs text-gray-600">
      <summary className="cursor-pointer select-none text-blue-700 hover:underline">{summary}</summary>
      <div className="mt-1 max-h-64 overflow-y-auto whitespace-pre-wrap break-words rounded-md border border-gray-100 bg-white p-2">
        {description.trim()}
      </div>
    </details>
  )
}
