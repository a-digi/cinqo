// modelJson.ts — the JSON a model wrote, read leniently (plan 209, step 3; moved here in plan 253, step 3, shared by the features that ask a model tool-free questions).
//
// A small model asked for "this JSON and nothing else" still wraps it in a code fence, adds a sentence before or after it, or stops short.
// This finds the first complete JSON object or list in its answer and reads it; it never repairs a broken one (a guess about what the model
// meant would be an invented fact), it says there is none.

// The first balanced {...} or [...] of the text, or null. Strings and their escapes are skipped when matching brackets.
function firstBalanced(text: string): string | null {
  const start = text.search(/[{[]/)
  if (start < 0) return null
  const stack: string[] = []
  let inString = false
  let escaped = false
  for (let i = start; i < text.length; i++) {
    const c = text.charAt(i)
    if (inString) {
      if (escaped) escaped = false
      else if (c === '\\') escaped = true
      else if (c === '"') inString = false
      continue
    }
    if (c === '"') inString = true
    else if (c === '{') stack.push('}')
    else if (c === '[') stack.push(']')
    else if (c === '}' || c === ']') {
      if (stack.pop() !== c) return null
      if (stack.length === 0) return text.slice(start, i + 1)
    }
  }
  return null
}

// The JSON value in a model's answer, or undefined when there is none (nothing JSON, or cut off, or not valid JSON).
export function parseModelJson(answer: string): unknown {
  const text = answer.trim()
  if (text === '') return undefined
  try {
    return JSON.parse(text)
  } catch {
    // not the whole answer: look for the first complete value inside it
  }
  const found = firstBalanced(text)
  if (found === null) return undefined
  try {
    return JSON.parse(found)
  } catch {
    return undefined
  }
}
