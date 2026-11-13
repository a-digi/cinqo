// The footnotes at the foot of a showcase page: small numbered grey lines. The card that refers to a note links to `#showcase-note-<n>` (WhyGrid).
export function Footnotes({ notes }: { notes: string[] }) {
  if (notes.length === 0) return null
  return (
    <ol data-testid="footnotes" className="mx-auto max-w-4xl list-none space-y-2 text-left text-xs leading-relaxed text-[#6e6e73]">
      {notes.map((n, i) => (
        <li key={n} id={`showcase-note-${String(i + 1)}`}>
          <sup className="mr-1">{i + 1}</sup>
          {n}
        </li>
      ))}
    </ol>
  )
}
