// The colours and sizes of the landing-page look (plan 267): a deep-navy hero band, large pale-grey rounded tiles, royal-blue outlined picture frames and
// black pill buttons. Kept in one place so every landing piece agrees. No React.
export const LANDING = {
  // The hero band.
  navy: '#0b1437',
  navySoft: '#16225c',
  // A tile and the page around the tiles.
  tile: '#f2f3f5',
  page: '#fafafa',
  // The outline of a picture frame and of the small round badges.
  outline: '#2f48ff',
  outlineSoft: '#e6e9ff',
  // Text and the black pill buttons.
  ink: '#0b0b0f',
  muted: '#5b6070',
} as const

export type LandingTone = 'dark' | 'light'
