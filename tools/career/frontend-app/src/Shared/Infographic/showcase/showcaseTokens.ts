// The colours and sizes of the showcase look (plan 269): a product-page manner: black and light-grey full-width bands, one blue for the filled pill, quiet grey text. Kept in one place so every
// showcase piece agrees. No React. The font is the system's own stack (no licensed font).
export const SHOWCASE = {
  black: '#000000',
  // the dark band's near-black and the text on a light band
  ink: '#1d1d1f',
  light: '#f5f5f7',
  white: '#ffffff',
  // secondary text on a light and on a dark band
  muted: '#6e6e73',
  mutedOnDark: '#a1a1a6',
  blue: '#0071e3',
  blueHover: '#0077ed',
  font: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif',
} as const

export type ShowcaseTone = 'dark' | 'light' | 'white'

// The background and the text colour of a band.
export function toneColors(tone: ShowcaseTone): { background: string; color: string; muted: string } {
  switch (tone) {
    case 'dark':
      return { background: SHOWCASE.black, color: SHOWCASE.white, muted: SHOWCASE.mutedOnDark }
    case 'light':
      return { background: SHOWCASE.light, color: SHOWCASE.ink, muted: SHOWCASE.muted }
    case 'white':
      return { background: SHOWCASE.white, color: SHOWCASE.ink, muted: SHOWCASE.muted }
  }
}
