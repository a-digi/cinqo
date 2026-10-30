import { PAGE_ACCENT } from './theme'

// A copy of the Career tool's cardStyle: a tool's build is self-contained, so the Browser tool
// keeps its own small copy of the shared look (see tools/career/frontend-app/src/Shared/cardStyle.ts).

// The rounded, softly blue tinted card the pages put their content in. Use both:
// <section className={cardClassName} style={cardStyle}>.
export const cardClassName = 'rounded-3xl border p-5 shadow-sm'
export const cardStyle = { borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }
