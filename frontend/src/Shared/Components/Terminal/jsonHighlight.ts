// Colouring for beautified JSON (plan 202), pure: a line is cut into pieces by kind, and pieces can be cut again where a search match
// starts and ends. Text only: the pieces carry a kind, the viewer maps it to a CSS class; nothing here makes HTML.

export type JsonKind = 'key' | 'string' | 'number' | 'literal' | 'punct'

export interface JsonToken {
  text: string
  // null: whitespace or anything the colouring leaves alone.
  kind: JsonKind | null
}

const LITERALS = new Set(['true', 'false', 'null'])

// Cuts one line of indented JSON into tokens that together give the line back exactly. A string followed by a colon is a key. A line that
// is only a piece of a longer one (cut at the window's byte limit) may end inside a string: the rest of the line is then that string.
export function tokenizeJsonLine(line: string): JsonToken[] {
  const out: JsonToken[] = []
  let i = 0
  const push = (text: string, kind: JsonKind | null) => {
    if (text !== '') out.push({ text, kind })
  }
  while (i < line.length) {
    const c = line[i]
    if (c === ' ' || c === '\t') {
      let j = i
      while (j < line.length && (line[j] === ' ' || line[j] === '\t')) j++
      push(line.slice(i, j), null)
      i = j
    } else if (c === '"') {
      let j = i + 1
      while (j < line.length) {
        if (line[j] === '\\') j += 2
        else if (line[j] === '"') {
          j++
          break
        } else j++
      }
      j = Math.min(j, line.length)
      let k = j
      while (k < line.length && (line[k] === ' ' || line[k] === '\t')) k++
      push(line.slice(i, j), line[k] === ':' ? 'key' : 'string')
      i = j
    } else if (c === '{' || c === '}' || c === '[' || c === ']' || c === ',' || c === ':') {
      push(c, 'punct')
      i++
    } else {
      let j = i
      while (j < line.length && !' \t"{}[],:'.includes(line[j])) j++
      const word = line.slice(i, j)
      push(word, LITERALS.has(word) ? 'literal' : /^-?\d/.test(word) ? 'number' : null)
      i = j
    }
  }
  return out
}

export interface JsonPiece {
  text: string
  kind: JsonKind | null
  // Part of a search match.
  hit: boolean
}

// The tokens of a line cut again at the edges of the search matches (parts: the line split into hit and non-hit pieces, together equal to
// the line). Every piece keeps its token's kind and says whether it is inside a match, so a match across tokens is still marked whole.
export function jsonPieces(tokens: JsonToken[], parts: readonly { text: string; hit: boolean }[]): JsonPiece[] {
  const out: JsonPiece[] = []
  let ti = 0
  let tOff = 0
  let pi = 0
  let pOff = 0
  while (ti < tokens.length && pi < parts.length) {
    const t = tokens[ti]
    const p = parts[pi]
    const take = Math.min(t.text.length - tOff, p.text.length - pOff)
    if (take <= 0) {
      // an empty token or part: step past it
      if (t.text.length - tOff <= 0) {
        ti++
        tOff = 0
      }
      if (p.text.length - pOff <= 0) {
        pi++
        pOff = 0
      }
      continue
    }
    out.push({ text: t.text.slice(tOff, tOff + take), kind: t.kind, hit: p.hit })
    tOff += take
    pOff += take
    if (tOff >= t.text.length) {
      ti++
      tOff = 0
    }
    if (pOff >= p.text.length) {
      pi++
      pOff = 0
    }
  }
  return out
}

// The class of each kind on the dark terminal. Every colour is at least 4.5:1 on the pane's background.
export const JSON_KIND_CLASS: Record<JsonKind, string> = {
  key: 'text-sky-300',
  string: 'text-emerald-300',
  number: 'text-amber-300',
  literal: 'text-fuchsia-300',
  punct: 'text-[#8b95a5]',
}
