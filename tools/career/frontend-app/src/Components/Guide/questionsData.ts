// The questions of the Questions page (plan/ai/tools/career/step-170-onboarding-guide.md): plain text, each answer taken from what the
// real pages say and do, checked by questionsRules.ts (every link is a page of paths.ts). Keep the wording in step with the pages.
import {
  CLEANUP_SETTINGS_PATH,
  COMPANIES_PATH,
  CRAWLER_SETTINGS_PATH,
  CRAWL_MONITOR_PATH,
  IMPORT_CV_PATH,
  JOBS_PATH,
  PERSONAS_PATH,
  PLATFORMS_PATH,
  PROFILE_IMAGE_PATH,
  RECRUITERS_PATH,
  SETTINGS_PATH,
} from '../../paths'
import type { Question } from './guideTypes'

export const QUESTIONS: Question[] = [
  {
    id: 'no-jobs',
    question: 'Why do I see no jobs?',
    answer:
      'Jobs come from two places: platform links that have been crawled, and jobs you add yourself. Check that a link has crawl instructions and was crawled with Crawl now, and look at the Crawl Monitor for a crawl that is still running. A link that was crawled but found nothing shows Crawled 0 jobs. If Ignore jobs is on, new jobs that fail your rules are not saved.',
    link: { label: 'Open Platforms', path: PLATFORMS_PATH },
  },
  {
    id: 'add-job',
    question: 'Can I add a job myself?',
    answer: 'Yes. Use Add job on the Jobs page. A job you add yourself is marked Manual.',
    link: { label: 'Open Jobs', path: JOBS_PATH },
  },
  {
    id: 'which-ai',
    question: 'Which AI platform and model is used?',
    answer:
      'One choice in Settings → AI is used by everything: crawling and the crawl instructions, Job Match, Generate CV for a job and Import from CV. If the AI fails on a link three times in a row, the link shows Probably can’t be crawled; then try another model, or add that link’s jobs by hand.',
    link: { label: 'Open Settings → AI', path: SETTINGS_PATH },
  },
  {
    id: 'match-ai',
    question: 'Does Job Match always use the AI?',
    answer:
      'No. A job without a match gets one from the manual engine, which uses no AI, by itself every few minutes. Choose Job Match in a job’s actions if you want the AI to judge it.',
    link: { label: 'Open Jobs', path: JOBS_PATH },
  },
  {
    id: 'stop-crawl',
    question: 'How do I stop a crawl?',
    answer: 'Open the Crawl Monitor and use Stop on that crawl. It interrupts the crawl on the server, not just in the view.',
    link: { label: 'Open Crawl Monitor', path: CRAWL_MONITOR_PATH },
  },
  {
    id: 'ignore-saved',
    question: 'Does Ignore jobs remove jobs I already have?',
    answer:
      'No. It is off until you switch it on, it only checks new jobs before they are saved, and jobs that are already saved are never removed or hidden by it.',
    link: { label: 'Open Settings → Job crawler', path: CRAWLER_SETTINGS_PATH },
  },
  {
    id: 'old-jobs',
    question: 'How do I get rid of old jobs?',
    answer:
      'Job cleanup deletes old jobs and jobs that match none of your profiles. Run it by hand with Clean up now, or on a schedule with Automatic cleanup.',
    link: { label: 'Open Settings → Job cleanup', path: CLEANUP_SETTINGS_PATH },
  },
  {
    id: 'delete-company',
    question: 'What happens when I delete a company?',
    answer: 'Its jobs are un-linked, never deleted. Its recruiters are never deleted either: the company just disappears from them.',
    link: { label: 'Open Companies', path: COMPANIES_PATH },
  },
  {
    id: 'several-companies',
    question: 'Can a recruiter work for several companies?',
    answer:
      'Yes. Pick as many companies as you like in New recruiter, and change them any time with Edit. A recruiter with no company is allowed too; the list then shows No company.',
    link: { label: 'Open Recruiters', path: RECRUITERS_PATH },
  },
  {
    id: 'persona',
    question: 'What is a persona?',
    answer:
      'One role a profile is pursuing, for example “Backend Engineer”. Each persona has its own summary, skills and experience, and the AI matches jobs and writes CVs from it.',
    link: { label: 'Open Personas', path: PERSONAS_PATH },
  },
  {
    id: 'import-review',
    question: 'Is everything saved straight away when I import a CV?',
    answer:
      'No. Import from CV lets the AI propose a profile, persona, summary, skills and experience, and you review everything before it is saved.',
    link: { label: 'Open Import from CV', path: IMPORT_CV_PATH },
  },
  {
    id: 'photos',
    question: 'Can a profile have several photos?',
    answer: 'Yes, with one of them as the default. The CV templates use them, and one photo can be shared by many profiles.',
    link: { label: 'Open Photo', path: PROFILE_IMAGE_PATH },
  },
]
