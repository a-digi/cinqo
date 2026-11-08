// A copy of the Career tool's plural(): a tool's build is self-contained, so the Browser tool
// keeps its own small copy of the shared look (see tools/career/frontend-app/src/Components/CompaniesPage/companyStats.ts).

// "3 domains", "1 domain".
export function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`
}
