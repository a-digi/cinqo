// The questions of the Questions page (plan/ai/tools/career/step-170-onboarding-guide.md): plain text, each answer taken from what the
// real pages say and do, checked by questionsRules.ts (every link is a page of paths.ts). Keep the wording in step with the pages.
import {
  APPLICATIONS_PATH,
  CLEANUP_SETTINGS_PATH,
  COMPANIES_PATH,
  CRAWLER_SETTINGS_PATH,
  CRAWL_MONITOR_PATH,
  EDUCATION_PATH,
  LANGUAGES_PATH,
  GOALS_PATH,
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
    id: 'why-login',
    question: 'Why does my platform ask me to log in?',
    answer:
      'Some sites show their jobs only to signed-in visitors. Tick Requires login on the link: Crawl now then opens a window at the site, you log in there yourself and press “I am logged in”, and the crawl reads the pages in that window. You can stop it at any time with Cancel.',
    link: { label: 'Open Platforms', path: PLATFORMS_PATH },
  },
  {
    id: 'login-stored',
    question: 'Is my login stored?',
    answer:
      'No. You type your login on the site itself, in a window that belongs to that one crawl. Nothing you type is seen or saved by Cinqo, and the window and everything in it are thrown away when the crawl ends, so the next crawl asks you again. For the same reason a link that needs a login is never crawled automatically.',
    link: { label: 'Open Platforms', path: PLATFORMS_PATH },
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
    id: 'log-emails',
    question: 'Where do I write down my emails to a recruiter?',
    answer:
      'Open the job and use its Protocol card: Add entry logs a message you sent or received, or a note, with its channel (e-mail, platform, SMS, WhatsApp, phone, video, in person or letter), the date, an optional time, the recruiter and the CV you sent. You enter it yourself, or ask the AI assistant to add it; nothing is read from your mailbox. The Applications page lists every job with a protocol and marks the ones waiting for a reply.',
    link: { label: 'Open Applications', path: APPLICATIONS_PATH },
  },
  {
    id: 'comments-or-protocol',
    question: 'Comments or protocol: which one do I use?',
    answer:
      'The protocol is a dated log of what was sent, received or planned in one application, with a channel and optionally the recruiter and the CV. Comments are free notes about a job, a profile, a company or a recruiter, with no date of your choosing. Use the protocol for the conversation and comments for everything else.',
    link: { label: 'Open Jobs', path: JOBS_PATH },
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
      'Yes. Pick as many companies as you like in New recruiter, and change them any time with Edit. A recruiter who works for no company can be marked Freelancer (switch it on in New recruiter or Edit); a freelancer has no companies, and the list shows a Freelancer chip. A recruiter whose last company was deleted shows No company instead.',
    link: { label: 'Open Recruiters', path: RECRUITERS_PATH },
  },
  {
    id: 'persona',
    question: 'What is a persona?',
    answer:
      'One role a profile is pursuing, for example “Backend Engineer”. A persona uses the profile’s summary and goals unless it has its own, chooses which of the profile’s skills and experience it uses (and can reword the experience for its role), and can have skills and experience of its own. The AI matches jobs and writes CVs from it.',
    link: { label: 'Open Personas', path: PERSONAS_PATH },
  },
  {
    id: 'goals',
    question: 'Are the summary and the goals per persona?',
    answer:
      'Both are set once for the profile: the summary on the Summary page, the goals (desired titles and locations, minimum salary, remote work) on the Goals page. A persona shows the profile’s value until you give that persona one of its own; the pages mark each as Following the profile or Own wording, and Use the profile’s value brings the shared one back. Headline and location are always the persona’s.',
    link: { label: 'Open Goals', path: GOALS_PATH },
  },
  {
    id: 'education',
    question: 'Where do I put my education, and do personas use it?',
    answer:
      'On the Education page. Schools and degrees belong to the profile itself and are shared by all its personas; a persona does not choose among them. An entry without an end date counts as ongoing. Short trainings and certificates go on the Courses page.',
    link: { label: 'Open Education', path: EDUCATION_PATH },
  },
  {
    id: 'languages-courses-hobbies',
    question: 'Where do I put my languages, courses and hobbies?',
    answer:
      'On their own pages under My Profile: Languages (spoken, written or both, with an optional level from C2 down to A1 or Native), Courses (short trainings and certificates, with an optional certificate link) and Hobbies. Like education, they belong to the profile itself and are shared by all its personas.',
    link: { label: 'Open Languages', path: LANGUAGES_PATH },
  },
  {
    id: 'import-review',
    question: 'Is everything saved straight away when I import a CV?',
    answer:
      'No. Import from CV lets the AI propose a profile, persona, summary, skills, experience and education (skills and experience go into the profile’s library, education into the profile; an education entry the profile already has is not added again), and you review everything before it is saved.',
    link: { label: 'Open Import from CV', path: IMPORT_CV_PATH },
  },
  {
    id: 'photos',
    question: 'Can a profile have several photos?',
    answer: 'Yes, with one of them as the default. The CV templates use them, and one photo can be shared by many profiles.',
    link: { label: 'Open Photo', path: PROFILE_IMAGE_PATH },
  },
]
