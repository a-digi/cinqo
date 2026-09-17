// Small, standalone module (not colocated in ImportCvPage.tsx) so both
// that page and ImportRunDetail.tsx's own read-only rendering can share
// the exact same experience label text without violating this
// codebase's react-refresh rule (a component file may only export
// components).
export function experienceLabel(item: { title: string; company: string; startDate: string; endDate: string }): string {
  const when = item.startDate || item.endDate ? ` (${item.startDate || '?'} – ${item.endDate || 'present'})` : ''
  return `${item.title} at ${item.company}${when}`
}
