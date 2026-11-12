// The sidebar icons of the Career menu (plan/ai/frontend/step-232-menu-icons.md): one glyph per item that says what its page does. Raw SVG,
// drawn through the host's allowlist sanitizer (svg, path, circle, rect, line... with d, viewBox, fill, stroke, stroke-width,
// stroke-linecap, stroke-linejoin, cx, cy, r, rx, ry, x, y, width, height only; at most 2000 characters), so each is a monochrome outline.
// Where a page has a header icon (Shared/Icons/icons.tsx) the menu icon is the same shape on the same 24 grid, so the menu and the page agree.

// A 24 grid outline: the shared stroke settings sit on the root, the paths only draw.
const g24 = (body: string) =>
  `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`
// A 20 grid outline (the finer glyphs of the app's own menu use this one).
const g20 = (body: string) =>
  `<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`

// A document with its folded corner, shared by the CV glyphs.
const DOC = '<path d="M6 2.5h8.5L19 7v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V3.5a1 1 0 0 1 1-1z"/><path d="M14.5 2.5V7H19"/>'

// --- Profiles ---
// Profiles: a person (the Profiles page's own header icon).
export const profilesIcon = g24('<circle cx="12" cy="8" r="3.6"/><path d="M4.5 20c.8-3.6 3.8-5.6 7.5-5.6s6.7 2 7.5 5.6"/>')
// Personas: two people.
export const personasIcon = g24(
  '<circle cx="9" cy="8.5" r="3.2"/><path d="M3 20c0-3.3 2.7-5.5 6-5.5s6 2.2 6 5.5M16 5.6a3.2 3.2 0 0 1 0 5.8M18 14.8c1.9.7 3 2.4 3 5.2"/>',
)
// Summary: a title block and lines of text.
export const summaryIcon = g24('<rect x="4" y="4.5" width="16" height="4" rx="1"/><path d="M5 13h14M5 17h9"/>')
// Goals: a target.
export const goalsIcon = g24('<circle cx="12" cy="12" r="8.5"/><circle cx="12" cy="12" r="4.8"/><circle cx="12" cy="12" r="1.2"/>')
// Skills: bars of different length (levels).
export const skillsIcon = g24(
  '<rect x="4" y="5" width="16" height="3.2" rx="1.6"/><rect x="4" y="10.4" width="11" height="3.2" rx="1.6"/><rect x="4" y="15.8" width="7" height="3.2" rx="1.6"/>',
)
// Experience: a timeline.
export const experienceIcon = g24(
  '<path d="M7 4v16"/><circle cx="7" cy="7" r="1.8"/><circle cx="7" cy="17" r="1.8"/><path d="M11.5 7H19M11.5 17H17"/>',
)
// Education: a graduation cap.
export const educationIcon = g24(
  '<path d="M12 4 2.5 9 12 14l9.5-5L12 4z"/><path d="M6 11.2V16c0 1.4 2.7 2.8 6 2.8s6-1.4 6-2.8v-4.8M21.5 9v6"/>',
)
// Languages: two speech bubbles.
export const languagesIcon = g24(
  '<path d="M3.5 5.5A1.5 1.5 0 0 1 5 4h8a1.5 1.5 0 0 1 1.5 1.5v5A1.5 1.5 0 0 1 13 12H8.5L5.5 15v-3H5a1.5 1.5 0 0 1-1.5-1.5v-5z"/><path d="M17.5 9H19a1.5 1.5 0 0 1 1.5 1.5v5A1.5 1.5 0 0 1 19 17h-.5v3l-3-3H11a1.5 1.5 0 0 1-1.5-1.5V14"/>',
)
// Courses: a certificate with a ribbon.
export const coursesIcon = g24(
  '<rect x="3.5" y="4" width="17" height="12" rx="1.5"/><path d="M7 8.5h10M7 11.5h6"/><path d="m9 16-1 4.5 3.5-1.8 3.5 1.8-1-4.5"/>',
)
// Hobbies: a star.
export const hobbiesIcon = g24('<path d="m12 3.8 2.5 5.1 5.6.8-4 4 1 5.6-5.1-2.7-5.1 2.7 1-5.6-4-4 5.6-.8L12 3.8z"/>')

// --- CV ---
// CV (group) and Profile & CV: a document with a person on it.
export const cvIcon = g24(`${DOC}<circle cx="12" cy="11.5" r="2"/><path d="M8.3 18c.4-1.7 1.9-2.6 3.7-2.6s3.3.9 3.7 2.6"/>`)
// CVs: a document with lines of text.
export const cvDocumentsIcon = g24(`${DOC}<path d="M8 13h8M8 16.2h8M8 9.8h4"/>`)
// Import: an arrow down into a tray.
export const importIcon = g24('<path d="M12 4.5v11M8.2 11.7 12 15.5l3.8-3.8"/><path d="M4.5 15.5v3a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2v-3"/>')

// --- Jobs ---
// Jobs (group): a briefcase with a magnifier (the jobs you look for).
export const jobsGroupIcon = g24(
  '<rect x="3" y="6.5" width="14" height="10.5" rx="2"/><path d="M7.5 6.5v-1A1.5 1.5 0 0 1 9 4h2a1.5 1.5 0 0 1 1.5 1.5v1M3 11.5h14"/><circle cx="17" cy="17" r="3"/><path d="m19.2 19.2 1.8 1.8"/>',
)
// Jobs: a briefcase.
export const jobsIcon = g24(
  '<rect x="3.5" y="7.5" width="17" height="12" rx="2.5"/><path d="M9 7.5V6a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v1.5M3.5 13h17"/>',
)
// Applications: a paper plane (sent).
export const applicationsIcon = g24('<path d="M21 3 10.5 13.5M21 3l-6.5 18-4-7.5L3 9.5 21 3z"/>')
// Platforms (group): stacked layers.
export const platformsGroupIcon = g24(
  '<path d="M12 3.5 3.5 8.2 12 12.9l8.5-4.7L12 3.5z"/><path d="M3.5 12.2 12 16.9l8.5-4.7M3.5 16.2 12 20.9l8.5-4.7"/>',
)
// Platforms: a globe (the sites the jobs come from).
export const platformsIcon = g24(
  '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.6 2.7 3.8 5.7 3.8 9s-1.2 6.3-3.8 9c-2.6-2.7-3.8-5.7-3.8-9S9.4 5.7 12 3z"/>',
)
// How platforms work: a question mark in a circle.
export const howPlatformsWorkIcon = g24(
  '<circle cx="12" cy="12" r="9"/><path d="M9.6 9.4a2.5 2.5 0 1 1 3.6 2.2c-.8.4-1.2.9-1.2 1.8M12 17h.01"/>',
)
// Crawl Monitor: a pulse line.
export const crawlMonitorIcon = g24('<path d="M3 12h4l2.5-6 4 12 2.5-6H21"/>')
// Contacts (group): an address book.
export const contactsIcon = g24(
  '<rect x="5" y="3.5" width="14" height="17" rx="2"/><circle cx="12" cy="10" r="2.4"/><path d="M8.2 16.5c.5-1.8 1.9-2.8 3.8-2.8s3.3 1 3.8 2.8M3 7.5h2M3 12h2M3 16.5h2"/>',
)
// Companies: a building.
export const companiesIcon = g24(
  '<path d="M5 21V5.5A1.5 1.5 0 0 1 6.5 4h7A1.5 1.5 0 0 1 15 5.5V21M15 10h3.5A1.5 1.5 0 0 1 20 11.5V21M3 21h18"/><path d="M8.5 8h3M8.5 12h3M8.5 16h3"/>',
)
// Recruiters: a person with a speech bubble (someone who talks to you).
export const recruitersIcon = g24(
  '<circle cx="7.5" cy="10" r="3"/><path d="M2 20c0-3.2 2.4-5.2 5.5-5.2s5.5 2 5.5 5.2"/><path d="M14.5 4h5A1.5 1.5 0 0 1 21 5.5v4a1.5 1.5 0 0 1-1.5 1.5H18l-2.5 2.5V11h-1A1.5 1.5 0 0 1 13 9.5v-4A1.5 1.5 0 0 1 14.5 4z"/>',
)

// --- Settings ---
// Settings (group): sliders.
export const settingsIcon = g24('<path d="M4 7h9M17 7h3M4 17h3M11 17h9"/><circle cx="15" cy="7" r="2"/><circle cx="9" cy="17" r="2"/>')
// AI: a robot.
export const aiSettingsIcon = g20(
  '<path d="M10 3v2M7.5 13h5M4.5 9.5H3.3M16.7 9.5h-1.2"/><circle cx="10" cy="4.2" r="0.7" fill="currentColor" stroke="none"/><rect x="4.5" y="6" width="11" height="9" rx="2"/><circle cx="7.5" cy="10.2" r="0.9" fill="currentColor" stroke="none"/><circle cx="12.5" cy="10.2" r="0.9" fill="currentColor" stroke="none"/>',
)
// Job cleanup: a trash can.
export const jobCleanupIcon = g20(
  '<path d="M4 6h12M8 6V4.5a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1V6"/><path d="M5.5 6l.6 9.2a1 1 0 0 0 1 .9h5.8a1 1 0 0 0 1-.9L14.5 6"/><path d="M8.3 9v4M11.7 9v4"/>',
)
// Job crawler: a filter funnel (which jobs are kept out).
export const jobCrawlerIcon = g20('<path d="M3 4h14l-5.5 6.5v4.5l-3 1.5v-6z"/>')
