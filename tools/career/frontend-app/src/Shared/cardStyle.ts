import { PAGE_ACCENT } from '../Components/JobsPage/jobsTheme'

// The rounded, softly blue tinted card the Career pages put their content in. Use
// both: <section className={cardClassName} style={cardStyle}>.
export const cardClassName = 'rounded-3xl border p-5 shadow-sm'
export const cardStyle = { borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }
