// How many characters (Unicode code points, as the server counts them) a text has: "é" is one, an emoji is one.
export function charCount(text: string): number {
  return Array.from(text).length
}
