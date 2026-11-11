// templateCategories.ts (plan 254, step 2) — the order of the style pills of the template picker. The pills used to take their order from the first appearance of each
// category in the list of designs; now that the list puts the two-column designs first, that would reorder the pills, so they keep a fixed order instead and only the design
// cards move. A category this list does not know (a design added later with a new one) comes after the known ones, in the order it appears.

export const CATEGORY_ORDER = ['Modern', 'Classic', 'ATS-Friendly', 'Creative', 'Executive', 'Minimalist'] as const

export function orderedCategories(templates: readonly { category: string }[]): string[] {
  const present = new Set(templates.map((t) => t.category))
  const known = CATEGORY_ORDER.filter((c) => present.has(c))
  const extra: string[] = []
  for (const t of templates) {
    if (!(CATEGORY_ORDER as readonly string[]).includes(t.category) && !extra.includes(t.category)) extra.push(t.category)
  }
  return [...known, ...extra]
}
