// The in-app explanation of the link crawler (plan 185, step 9): what it does, what it needs, how to
// set it up, and what happens to the data of each job afterwards. It is shown open in the right column of the
// instruction modal (plan 246); every statement here is a rule the code enforces.
export function LinkCrawlHelp() {
  return (
    <section className="text-xs leading-relaxed text-gray-700" data-testid="link-crawl-help">
      <h3 className="text-sm font-semibold text-gray-900">How the link crawler works</h3>
      <div className="mt-3 space-y-3">
        <p>
          The list of jobs is found <strong>without AI</strong>: the Browser tool reads the page, and the jobs are the links whose address
          matches your rule. The title of a job is the text of its link.
        </p>
        <div>
          <div className="font-semibold text-gray-900">What it needs</div>
          <ul className="list-disc pl-5">
            <li>
              <strong>Katana</strong> must be the selected engine (the default, on the Browser &amp; Engine page). The rule is what the link
              is crawled with while Katana is selected; with Rod selected the link is crawled with its AI instructions instead, and a link
              without a rule is never crawled with them under Katana.
            </li>
            <li>The link must not need a login: links behind a login are crawled by hand only.</li>
          </ul>
        </div>
        <div>
          <div className="font-semibold text-gray-900">Setting it up</div>
          <ol className="list-decimal pl-5">
            <li>Load the links of the page, then tick two or three that are jobs.</li>
            <li>
              Mark a link that looks like a job but is not one with <em>not a job</em>, and the link to the next page of the list with{' '}
              <em>next page</em>.
            </li>
            <li>Check the proposed rule and its preview (how many jobs it finds), run the test, then save.</li>
          </ol>
        </div>
        <div>
          <div className="font-semibold text-gray-900">Reading a rule</div>
          <ul className="list-disc pl-5 font-mono">
            <li>host/jobs/* — one part of the address after /jobs/</li>
            <li>host/jobs/# — a number after /jobs/ (not a word such as /jobs/types)</li>
            <li>host/jobs/#-* — a number, a dash and a name</li>
            <li>host/careers/** — anything below /careers/</li>
          </ul>
        </div>
        <div>
          <div className="font-semibold text-gray-900">After the list: the job details</div>
          <p>
            A job found this way starts with its title and address only. The job-detail step (the AI-written detail instructions, then{' '}
            <em>Crawl job details now</em>) adds the description and fills the company, the location and the posted date{' '}
            <strong>only where they are empty</strong>. It never changes the title, never overwrites a value you or an earlier crawl set,
            and never touches jobs you added by hand. A company or location that contains the job&apos;s own title is ignored, and a posted
            date is saved as <span className="font-mono">YYYY-MM-DD</span>.
          </p>
        </div>
        <p>
          A job you delete stays deleted: the crawler does not add it again. Two postings with the same title but different addresses are
          two jobs.
        </p>
      </div>
    </section>
  )
}
