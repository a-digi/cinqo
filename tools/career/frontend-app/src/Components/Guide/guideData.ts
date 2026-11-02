// The guides of the Q&A menu (plan/ai/tools/career/step-170-onboarding-guide.md): plain text, written from what the real pages say and
// do, and checked by guideRules.ts (every link is a page of paths.ts). Keep the wording in step with the pages it describes.
import {
  CLEANUP_SETTINGS_PATH,
  COMPANIES_PATH,
  CRAWLER_SETTINGS_PATH,
  CRAWL_MONITOR_PATH,
  CV_DOCUMENTS_PATH,
  EDUCATION_PATH,
  EXPERIENCE_PATH,
  GOALS_PATH,
  IMPORT_CV_PATH,
  JOBS_PATH,
  PERSONAS_PATH,
  PERSONA_DETAILS_PATH,
  PLATFORMS_PATH,
  PROFILES_PATH,
  PROFILE_IMAGE_PATH,
  RECRUITERS_PATH,
  SETTINGS_PATH,
  SKILLS_PATH,
} from '../../paths'
import type { Guide } from './guideTypes'

export const GETTING_STARTED: Guide = {
  id: 'getting-started',
  design: 'zigzag',
  title: 'Getting started',
  description: 'The whole journey in seven steps, from choosing the AI to a CV for a job you want. Each step opens the page it is about.',
  steps: [
    {
      id: 'choose-ai',
      title: 'Choose the AI',
      text: 'Pick the AI platform and model that every Career feature uses: crawling job platforms, Job Match and CVs.',
      link: { label: 'Open Settings → AI', path: SETTINGS_PATH },
    },
    {
      id: 'create-profile',
      title: 'Create your profile',
      text: 'A profile is one job seeker. Personas, skills, experience and CVs all belong to a profile, so create it first with New profile.',
      link: { label: 'Open Profiles', path: PROFILES_PATH },
    },
    {
      id: 'fill-in',
      title: 'Tell the tool who you are',
      text: 'Let the AI read your CV with Import from CV (you review everything before it is saved), or add your personas, skills and experience by hand.',
      tip: 'A persona is a role you are looking for. Add one for each role.',
      link: { label: 'Open Import from CV', path: IMPORT_CV_PATH },
    },
    {
      id: 'add-platforms',
      title: 'Add the job platforms',
      text: 'On Platforms, create a platform with New platform, add a link to one of its search pages with Add link, and let the AI write the crawl instructions with Generate with AI. Crawl now then fetches the jobs.',
      tip: 'The Crawl Monitor lists every crawl that is running right now.',
      link: { label: 'Open Platforms', path: PLATFORMS_PATH },
    },
    {
      id: 'look-at-jobs',
      title: 'Look through the jobs',
      text: 'Saved Jobs lists every job the crawler saved and any you added yourself. Each gets a match score; a job without one gets it by itself every few minutes, or run Job Match from the job’s actions. View details opens the whole posting.',
      link: { label: 'Open Jobs', path: JOBS_PATH },
    },
    {
      id: 'follow-up',
      title: 'Keep track of the jobs you care about',
      text: 'Use Add to job board in a job’s actions to follow it with a status; a profile’s page shows its Job board. Companies and Recruiters hold the people behind the jobs, and Recruiter… in a job’s actions maps a recruiter to it.',
      link: { label: 'Open Profiles', path: PROFILES_PATH },
    },
    {
      id: 'build-cv',
      title: 'Build a CV for a job',
      text: 'Pick a persona and create a CV from its data in one of a few designs. The CV action of a job builds one for that job.',
      link: { label: 'Open CV Documents', path: CV_DOCUMENTS_PATH },
    },
  ],
}

export const PROFILE_AND_CV: Guide = {
  id: 'profile-and-cv',
  design: 'cards',
  title: 'Profile & CV',
  description: 'How to set up who you are and turn it into a CV, in ten steps. The AI reads all of it when it matches jobs and writes CVs.',
  steps: [
    {
      id: 'profile',
      title: 'Create the profile',
      text: 'A profile is one job seeker. Personas, skills, experience and education hang off it, so create it first with New profile; you can add links afterwards.',
      link: { label: 'Open Profiles', path: PROFILES_PATH },
    },
    {
      id: 'import',
      title: 'Or import it from your CV',
      text: 'Short on time? Upload a CV with Import from CV. The AI proposes a profile, a persona, a summary, skills and experience from it, and you review everything before it is saved.',
      tip: 'Some models are sometimes not good enough and miss things, but they can still do the job. That is why you check what is proposed before you save it.',
      link: { label: 'Open Import from CV', path: IMPORT_CV_PATH },
    },
    {
      id: 'persona',
      title: 'Add a persona for each role',
      text: 'A persona is one role the profile is pursuing, for example “Backend Engineer”. Each uses the profile’s summary and goals unless it has its own, and chooses which skills and experience it uses. Pick the profile, then use New persona.',
      link: { label: 'Open Personas', path: PERSONAS_PATH },
    },
    {
      id: 'summary',
      title: 'Write the summary',
      text: 'The profile has one summary; write it at the top of the Summary page. A persona shows it until you open the persona and word its own, and its headline and location are always its own. The AI reads it when matching jobs and writing CVs.',
      link: { label: 'Open Summary', path: PERSONA_DETAILS_PATH },
    },
    {
      id: 'goals',
      title: 'Set the goals',
      text: 'Desired titles, desired locations, a minimum salary and remote work are set once for the profile on the Goals page. A persona follows them unless you give it a goal of its own; each persona’s card says which is which.',
      link: { label: 'Open Goals', path: GOALS_PATH },
    },
    {
      id: 'skills',
      title: 'Add the skills',
      text: 'The profile keeps one library of skills. Add them there, then pick a persona to choose which ones it uses, or to give it skills of its own. The AI reads them when matching you against job postings.',
      link: { label: 'Open Skills', path: SKILLS_PATH },
    },
    {
      id: 'experience',
      title: 'Add the work experience',
      text: 'The profile keeps one library of work history. Add entries there, then pick a persona to choose which ones it uses, reword their title or description for that role, or add entries of its own. The AI reads it when matching you against job postings.',
      link: { label: 'Open Experience', path: EXPERIENCE_PATH },
    },
    {
      id: 'education',
      title: 'Add the education',
      text: 'Schools, degrees and courses belong to the profile itself and are shared by all its personas. Add one with Add education; an entry without an end date counts as ongoing.',
      link: { label: 'Open Education', path: EDUCATION_PATH },
    },
    {
      id: 'photo',
      title: 'Add a photo (optional)',
      text: 'A profile can have several photos, one of them the default, and the CV templates use them. Add one with Upload photo.',
      link: { label: 'Open Photo', path: PROFILE_IMAGE_PATH },
    },
    {
      id: 'cv',
      title: 'Create a CV',
      text: 'Pick a persona and use New CV to create a CV from its own data, in one of a few fixed designs.',
      link: { label: 'Open CV Documents', path: CV_DOCUMENTS_PATH },
    },
  ],
}

export const FINDING_JOBS: Guide = {
  id: 'finding-jobs',
  design: 'pins',
  title: 'Finding jobs',
  description:
    'How jobs get into the tool and how to keep the list useful, in seven steps: platforms, crawling, filtering, cleaning up and matching.',
  steps: [
    {
      id: 'ai',
      icon: 'robot',
      title: 'Choose the AI',
      text: 'Crawling job platforms, Job Match and CVs all use the one AI platform and model chosen in Settings. Pick it before anything else.',
      link: { label: 'Open Settings → AI', path: SETTINGS_PATH },
    },
    {
      id: 'platform',
      icon: 'globe',
      title: 'Add a platform and a link',
      text: 'A platform is a job site. Create one with New platform, then add a link to one of its search pages with Add link.',
      link: { label: 'Open Platforms', path: PLATFORMS_PATH },
    },
    {
      id: 'instructions',
      icon: 'instructions',
      title: 'Let the AI write the crawl instructions',
      text: 'A link can only be crawled once it has crawl instructions. Generate with AI writes them: the AI studies the page and works out how to read its job list. Job details have their own instructions, so a posting can be read in full.',
      tip: 'Not every model is good enough for this: some fail or write poor instructions. Premium models do a better job. If a link keeps failing, pick a premium model in Settings → AI and try again.',
      link: { label: 'Open Platforms', path: PLATFORMS_PATH },
    },
    {
      id: 'crawl',
      icon: 'pulse',
      title: 'Crawl and watch it work',
      text: 'Crawl now fetches the jobs and saves them. The Crawl Monitor lists every crawl that is running right now and refreshes by itself.',
      tip: 'Auto-discovery at the top of the Platforms page crawls every link that has instructions on a schedule (Crawl every …).',
      link: { label: 'Open Crawl Monitor', path: CRAWL_MONITOR_PATH },
    },
    {
      id: 'ignore',
      icon: 'filter',
      title: 'Keep unwanted jobs out',
      text: 'Ignore jobs lets the crawler check each new job against your rules before it saves it. It is off until you switch it on, and jobs that are already saved are never removed by it.',
      link: { label: 'Open Settings → Job crawler', path: CRAWLER_SETTINGS_PATH },
    },
    {
      id: 'cleanup',
      icon: 'trash',
      title: 'Clean up old jobs',
      text: 'Job cleanup deletes old jobs and jobs that match none of your profiles, by hand with Clean up now or on a schedule with Automatic cleanup.',
      link: { label: 'Open Settings → Job cleanup', path: CLEANUP_SETTINGS_PATH },
    },
    {
      id: 'match',
      icon: 'match',
      title: 'Find the best matches',
      text: 'Every job gets a match score against your profiles. Sort by Best match first, run Job Match from a job’s actions to have the AI judge it, and open View details for the whole posting.',
      link: { label: 'Open Jobs', path: JOBS_PATH },
    },
  ],
}

export const CONTACTS: Guide = {
  id: 'contacts',
  design: 'radial',
  title: 'Companies & recruiters',
  description: 'Keep track of the companies and the recruiters behind your jobs, in five steps, and link them to the jobs they belong to.',
  steps: [
    {
      id: 'companies',
      icon: 'building',
      title: 'Add the companies',
      text: 'Companies is a directory, separate from the free-text name on each crawled job. Add one with New company; you can link jobs and recruiters to it afterwards. Use View on a company to see its page.',
      link: { label: 'Open Companies', path: COMPANIES_PATH },
    },
    {
      id: 'recruiters',
      icon: 'users',
      title: 'Add the recruiters',
      text: 'A recruiter works for one or more existing companies, so add the companies first, or is a freelancer who works for none. Create one with New recruiter and pick the companies, or switch on Freelancer; Edit changes it later.',
      tip: 'Deleting a company never deletes a recruiter: the company just disappears from them.',
      link: { label: 'Open Recruiters', path: RECRUITERS_PATH },
    },
    {
      id: 'map',
      icon: 'link',
      title: 'Map a recruiter to a job',
      text: 'In the Jobs list, open a job’s actions and choose Recruiter…, then pick one in the window: recruiters who work for the job’s company come first, marked Same company, and a freelancer shows Freelancer instead of a company. A job’s own page has a Recruiters card with Map to recruiter and a Remove for each recruiter already mapped.',
      link: { label: 'Open Jobs', path: JOBS_PATH },
    },
    {
      id: 'recruiter-page',
      icon: 'eye',
      title: 'See what a recruiter is mapped to',
      text: 'View on a recruiter opens their page: the e-mail, the companies they work for (or Freelancer) and every job they are mapped to, each with Remove from this job.',
      link: { label: 'Open Recruiters', path: RECRUITERS_PATH },
    },
    {
      id: 'company-recruiters',
      icon: 'briefcase',
      title: 'See who works at a company',
      text: 'A company’s page shows how many recruiters it has; the number opens the Recruiters list filtered to that company.',
      link: { label: 'Open Companies', path: COMPANIES_PATH },
    },
  ],
}
