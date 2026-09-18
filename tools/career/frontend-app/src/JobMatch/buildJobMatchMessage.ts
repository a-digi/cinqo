// buildJobMatchMessage.ts mirrors generateInstructions.ts's own shape
// exactly, for a different goal: assessing how well one job matches a
// career profile and recording a 0-100 score (job_match.go), never
// crawling a listing or writing crawl instructions. The conversation
// this message is sent to is created hidden (coreApi.ts's own
// createConversation, hidden: true) — it never shows up in the normal
// conversation list, so this reply is never read by any user; the
// Jobs page only ever surfaces the resulting score/failure via
// job.matchScore/matchStatus/matchError. See
// plan/ai/tools/career/step-XX-job-match.md.
export function buildJobMatchMessage(job: { id: string; title: string; company: string }, profileId: string): string {
  return [
    `Please assess how well this job posting matches career profile ${profileId}, and record a match score — do not ask which job or profile is meant, both are already specified below.`,
    '',
    `Job ID: ${job.id}`,
    `Title: ${job.title}`,
    `Company: ${job.company || '(unknown)'}`,
    `Profile ID: ${profileId}`,
    '',
    'Steps:',
    '1. Call get_job with this job id to read its current description.',
    "2. If the description is empty: call get_portal_link_job_detail_crawl_instructions for this job's own portalLinkId (from get_job's own result, if it has one). If instructions are set, call crawl_urls_with_subagents for this ONE job's own sourceUrl, using those same extractFields, saveToolName \"save_job_detail_extraction\", saveToolArgsKey \"jobId\", and saveToolArgsByUrl mapping that url to this job's own id — then poll check_subagent (or list_subagents) until it finishes, then call get_job again to see whether a description was produced. If there are no job detail crawl instructions for this link, or the description is still empty afterward, proceed anyway using only the job's own title/company/location — never refuse to produce a score just because there is no description.",
    `3. Call list_personas with profileId ${profileId} to see every persona under this profile.`,
    '4. For each persona, call get_persona_details to read its own skills, experience, and personal details (headline, summary, desired titles/locations, minimum salary).',
    "5. Decide which ONE persona is the best fit for this specific job, and how well it matches overall, as a single integer 0-100 (100 = perfect match) — weigh the job's own actual requirements (from its description/title) against that persona's own skills and experience realistically, not generously.",
    `6. Call save_job_match ONCE with jobId ${job.id}, profileId ${profileId}, the best-fit persona's own id, and your score.`,
    '',
    'When done, reply with a short one-line confirmation — this reply is never shown to any user, so keep it brief.',
  ].join('\n')
}

export function jobMatchConversationTitle(job: { title: string; company: string }): string {
  return `Job match: ${job.title}${job.company ? ` @ ${job.company}` : ''}`
}
