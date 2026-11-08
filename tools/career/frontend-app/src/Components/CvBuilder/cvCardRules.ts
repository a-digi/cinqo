// Pure rules for a CV card. No React, so they can be script-checked.
import type { CvTemplate } from '../../api'

// The template's name, or its id when the list has no such template (a removed design).
export function templateLabel(templates: Pick<CvTemplate, 'id' | 'name'>[], id: string): string {
  return templates.find((t) => t.id === id)?.name ?? id
}

// A short local date ("30 Oct 2026"), or '' when the timestamp cannot be read.
export function createdLabel(iso: string | undefined): string {
  if (!iso) return ''
  const date = new Date(iso)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

export type PreviewState = 'pdf' | 'template' | 'placeholder'

// What a card's thumbnail shows. The CV's own first page when it was rendered; else the
// template's screenshot (while the render is still running, when the host cannot render, or
// when the PDF cannot be previewed); else a plain file glyph. "sample" says the picture is the
// design, not this CV, so the card can label it once the render has given up.
export function previewState(args: { rendered: string | null; templateImage: string | undefined }): PreviewState {
  if (args.rendered) return 'pdf'
  return args.templateImage ? 'template' : 'placeholder'
}

// Which cards fit in a row: the grid's minimum column width, in rem (Media uses 15). A CV is
// a narrower page, so a few more columns fit.
export const CARD_MIN_REM = 13
