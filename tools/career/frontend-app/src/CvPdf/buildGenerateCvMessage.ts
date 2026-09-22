// buildGenerateCvMessage.ts mirrors buildJobMatchMessage.ts's own
// shape exactly, for a different goal: rendering a CV (as a PDF, via
// pdf_tools' own generate_pdf) tailored to how well a chosen profile's
// personas fit one specific job, then recording the result
// (jobs/cv_pdf.go's own save_cv_pdf). The conversation this message is
// sent to is created hidden (conversation.ts's own createConversation,
// hidden: true) — it never shows up in the normal conversation list,
// so this reply is never read by any user; the Jobs page only ever
// surfaces the resulting cvStatus/cvMediaFileId/cvError. See
// plan/ai/tools/career/step-XX-cv-pdf.md.
//
// generate_pdf accepts XHTML only (no markdown path) — the embedded
// skeleton below keeps every generated CV visually consistent
// (professional, one page, no external assets) rather than leaving
// layout entirely up to the model's own judgment call each time.
const XHTML_SKELETON = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
  <head>
    <meta charset="UTF-8" />
    <style>
      body { font-family: Helvetica, Arial, sans-serif; color: #111; font-size: 11pt; margin: 32px; }
      h1 { font-size: 20pt; margin: 0 0 2px 0; }
      .headline { font-size: 12pt; color: #444; margin: 0 0 14px 0; }
      h2 { font-size: 12pt; text-transform: uppercase; letter-spacing: 0.05em; border-bottom: 1px solid #ccc; padding-bottom: 3px; margin: 16px 0 8px 0; }
      .entry { margin-bottom: 10px; }
      .entry-title { font-weight: bold; }
      .entry-meta { color: #555; font-size: 9.5pt; }
      ul { margin: 4px 0 0 0; padding-left: 18px; }
      .skills span { display: inline-block; border: 1px solid #ccc; border-radius: 10px; padding: 2px 8px; margin: 0 4px 4px 0; font-size: 9.5pt; }
    </style>
  </head>
  <body>
    <h1>{{full name}}</h1>
    <p class="headline">{{headline tailored to this job}}</p>
    <h2>Summary</h2>
    <p>{{2-4 sentence summary, rewritten to foreground the experience/skills this specific job cares about most}}</p>
    <h2>Experience</h2>
    <!-- one .entry per relevant role, most recent first; omit or trim roles that don't help this job -->
    <h2>Skills</h2>
    <p class="skills"><span>{{skill}}</span> ...</p>
  </body>
</html>`

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
    "4. Decide which ONE persona is the best fit for this specific job (same judgment call as Job Match's own save_job_match), and build the CV from THAT persona's own real data only — never invent a skill, employer, title, or dates that aren't already present in its own get_persona_details result.",
    "5. Tailor the content, not just select it: rewrite the headline and summary to foreground exactly the experience and skills this job's own description/title actually asks for; reorder or trim experience entries so the most relevant ones lead; the final CV should read as if written specifically for this posting, not a generic dump of everything on file.",
    '6. Render the CV as XHTML matching this exact structure and inline styling (fill in every {{...}} placeholder with real content; keep the rest of the markup and CSS as-is so every generated CV stays visually consistent; keep it to one page — trim content rather than shrinking fonts if it runs long):',
    '',
    '```xhtml',
    XHTML_SKELETON,
    '```',
    '',
    "7. Call generate_pdf with that XHTML as the xhtml argument, then follow generate_pdf's own instructions for verifying it (via pdf_to_markdown) and fixing/regenerating if needed before proceeding.",
    '8. Only once verified, call save_cv_pdf ONCE, passing this job id and the exact `uri` field from that LAST, verified generate_pdf call, unmodified.',
    '',
    'When done, reply with a short one-line confirmation — this reply is never shown to any user, so keep it brief.',
  ].join('\n')
}

export function generateCvConversationTitle(job: { title: string; company: string }): string {
  return `Generate CV: ${job.title}${job.company ? ` @ ${job.company}` : ''}`
}
