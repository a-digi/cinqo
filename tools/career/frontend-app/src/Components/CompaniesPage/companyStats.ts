// Small pure helpers shared by the list pages (the overview of the Companies list itself comes from the server, step 137).

// "3 companies", "1 company".
export function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`
}

// A whole percent of part in total, or null when there is no total (so the ring shows a dash).
export function percentOrNull(part: number, total: number): number | null {
  return total <= 0 ? null : Math.round((part / total) * 100)
}
