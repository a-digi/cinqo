// buildGenerateCvMessage.ts mirrors buildJobMatchMessage.ts's own shape
// exactly, for a different goal: rendering a CV (as a PDF, via the CV
// Builder's own template system — cvbuilder's render_cv_document, NOT
// hand-authored XHTML) tailored to how well a chosen profile's personas
// fit one specific job, then recording the result as a real
// cv_documents row (jobs/cv_pdf.go's own save_cv_document). The
// conversation this message is sent to is created hidden
// (conversation.ts's own createConversation, hidden: true) — it never
// shows up in the normal conversation list, so this reply is never read
// by any user; the Jobs page only ever surfaces the resulting
// cvStatus/cvMediaFileId/cvError. See
// plan/ai/career/cv-builder/step-08-ai-generated-cv-documents.md.
//
// Replaces the earlier version of this prompt, which had the AI
// hand-author a fixed XHTML/CSS skeleton itself — that skeleton is gone
// entirely; the AI now supplies only structured data (via
// get_persona_cv_defaults + render_cv_document), and this tool's own
// 26 template designs handle all layout/styling. No pdf_to_markdown
// verification step between generate_pdf and save_cv_document either
// (there used to be one) — that existed to catch the AI's own
// hand-authored markup coming out malformed, which structurally can't
// happen once rendering goes through render_cv_document's own
// html/template pipeline (free text is auto-escaped). Removed since it
// was adding a real round trip to every single generation for a risk
// that no longer exists on this path.
export function buildGenerateCvMessage(job: { id: string; title: string; company: string }, profileId: string): string {
  return [
    `Please build a CV, tailored specifically to this job posting, for career profile ${profileId}, render it as a PDF, and record the result — do not ask which job or profile is meant, both are already specified below.`,
    '',
    `Job ID: ${job.id}`,
    `Title: ${job.title}`,
    `Company: ${job.company || '(unknown)'}`,
    `Profile ID: ${profileId}`,
    '',
    'Steps:',
    '1. Call get_job with this job id to read its current title/company/location/description — this is the job you are tailoring the CV to.',
    `2. Call list_personas with profileId ${profileId} to see every persona under this profile.`,
    '3. For each persona, call get_persona_details to read its own skills, experience, and personal details (headline, summary, desired titles/locations).',
    "4. Decide which ONE persona is the best fit for this specific job (same judgment call as Job Match's own save_job_match).",
    "5. Call list_cv_templates and pick ONE design well-suited to this persona/job (consider each template's own category and bestFor list).",
    '6. Call get_persona_cv_defaults for that persona — this gives you a real starting point (name, headline, summary, location, external links, skills, experience) built from its own actual data.',
    "7. Tailor that data for this specific job before rendering — do not just pass it through unchanged: rewrite the headline and summary to foreground exactly the experience and skills this job's own description/title asks for, and reorder or trim the skills/experience lists so the most relevant ones lead. Never invent a skill, employer, title, or dates that weren't already present in get_persona_cv_defaults' own result. Every field you write (fullName/headline/summary/location/skills/experience) must be PLAIN TEXT ONLY — no HTML tags or markup of any kind (no <b>, <strong>, <p>, <div>, <br>, etc.); render_cv_document handles all visual styling itself and will reject a field containing one.",
    '8. Call render_cv_document with that persona id, your chosen template id, and the tailored data — this returns real XHTML, already fully styled; you never write any HTML/CSS yourself.',
    '9. Call generate_pdf with that returned XHTML as the xhtml argument, completely UNCHANGED from what render_cv_document returned — do not edit, reformat, retype, or write any of your own XHTML for this task under any circumstances. If you find yourself typing an <html> tag by hand at any point in this flow, stop — you are doing it wrong.',
    '10. Immediately call save_cv_document ONCE, passing: this job id, the persona id, the template id, a short human-readable title for the CV (e.g. "{persona name} - {job title}"), the exact SAME tailored data object you passed to render_cv_document, and the `uri` field from that generate_pdf call, unmodified — no verification step needed in between.',
    '',
    'When done, reply with a short one-line confirmation — this reply is never shown to any user, so keep it brief.',
  ].join('\n')
}

export function generateCvConversationTitle(job: { title: string; company: string }): string {
  return `Generate CV: ${job.title}${job.company ? ` @ ${job.company}` : ''}`
}
