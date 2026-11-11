// cvExportRules.ts (plan 251, steps 5 and 6) — the pure parts of downloading a CV as a file: the name the file is saved under. The backend names it in Content-Disposition;
// when that header does not come through, the name is made from the CV's title by the same rule (letters, digits, spaces, dashes and underscores).

export type CvExportFormat = 'md' | 'docx'

export const EXPORT_EXTENSION: Record<CvExportFormat, string> = { md: 'md', docx: 'docx' }

// A title as a file name: only letters, digits, spaces, dashes and underscores, spaces collapsed, at most 100 characters, "cv" when nothing is left.
export function safeBaseName(title: string): string {
  const kept = Array.from(title.replace(/\s/g, ' '))
    .filter((c) => /[\p{L}\p{N}\-_ ]/u.test(c))
    .join('')
  const name = kept.split(' ').filter(Boolean).join(' ')
  const cut = Array.from(name).slice(0, 100).join('').trim()
  return cut === '' ? 'cv' : cut
}

// The name from a Content-Disposition header: the UTF-8 form (filename*=UTF-8''…) first, then the plain one. A name with a path, a control character or nothing
// before the extension is not trusted: the title makes the name then.
export function exportFileName(disposition: string | null, title: string, format: CvExportFormat): string {
  const ext = EXPORT_EXTENSION[format]
  const fallback = `${safeBaseName(title)}.${ext}`
  if (disposition === null) return fallback
  let name = ''
  const star = /filename\*\s*=\s*UTF-8''([^;]+)/i.exec(disposition)
  if (star) {
    try {
      name = decodeURIComponent(star[1].trim())
    } catch {
      name = ''
    }
  }
  if (name === '') {
    const plain = /filename\s*=\s*"([^"]*)"/i.exec(disposition)
    if (plain) name = plain[1]
  }
  const control = Array.from(name).some((c) => c.charCodeAt(0) < 32)
  if (control || /[/\\]/.test(name) || !name.toLowerCase().endsWith(`.${ext}`) || name.startsWith('.')) return fallback
  return name
}
