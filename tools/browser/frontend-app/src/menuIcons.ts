// The sidebar icons of the Browser menu items below the tool's own (plan/ai/frontend/step-232-menu-icons.md): one glyph per item that says what
// its page does. Raw SVG, drawn through the host's allowlist sanitizer (svg, path, circle, ellipse, rect, line... with d, viewBox, fill,
// stroke, stroke-width, stroke-linecap, stroke-linejoin, cx, cy, r, rx, ry, x, y, width, height only; at most 2000 characters), so each is a
// monochrome outline on a 20 grid like the tool's own globe.
const g20 = (body: string) =>
  `<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`

// Browser & Engine: a browser window with a play triangle (the engine that opens pages).
export const browserEngineIcon = g20('<rect x="3" y="4" width="14" height="12" rx="1.5"/><path d="M3 7.5h14"/><path d="M8.5 10.2v3.6l3-1.8-3-1.8z"/>')
// Challenges: a shield with an exclamation mark (a check only a person can pass).
export const challengesIcon = g20(
  '<path d="M10 3l6 2.2v4.3c0 4-2.6 6.8-6 7.5-3.4-.7-6-3.5-6-7.5V5.2L10 3z"/><path d="M10 7.5v3.5M10 13.3v.1"/>',
)
// Domains: a padlock (the sites that asked for a check).
export const domainsIcon = g20('<rect x="5" y="9" width="10" height="7.5" rx="1.5"/><path d="M7.2 9V7a2.8 2.8 0 0 1 5.6 0v2"/><path d="M10 12v1.5"/>')
// Debug (group): a bug.
export const debugGroupIcon = g20(
  '<path d="M8 6.5a2 2 0 0 1 4 0"/><ellipse cx="10" cy="11.5" rx="3.5" ry="4.5"/><path d="M10 7v9M6.5 10H3.5M16.5 10h-3M6.5 13.5L4 15M13.5 13.5L16 15M6.8 8.2L4.5 6.8M13.2 8.2l2.3-1.4"/>',
)
// Debug: sliders (switches the logging on and off).
export const debugIcon = g20('<path d="M3.5 6h8M15 6h1.5M3.5 14h1.5M9 14h7.5"/><circle cx="13" cy="6" r="1.8"/><circle cx="7" cy="14" r="1.8"/>')
// Crawl Logs: a document with lines and a clock (the history of crawls).
export const crawlLogsIcon = g20(
  '<path d="M9 17H5a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h6l3.5 3.5V8.5"/><path d="M7 8h4M7 11h2.5"/><circle cx="14" cy="14" r="3.2"/><path d="M14 12.4V14l1.2.8"/>',
)
