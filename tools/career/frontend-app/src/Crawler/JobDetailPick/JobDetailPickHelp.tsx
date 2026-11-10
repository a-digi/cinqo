// The in-app explanation of "Pick the data of a job page" (plan 186, step 12): what it does, how to use it, what saving does to jobs and what
// it needs. Shown open in the right column of the instruction modal (plan 246); every statement here is a rule the code enforces.
export function JobDetailPickHelp() {
  return (
    <section className="text-xs leading-relaxed text-gray-700" data-testid="job-detail-pick-help">
      <h3 className="text-sm font-semibold text-gray-900">How picking the data of a job page works</h3>
      <div className="mt-3 space-y-3">
        <p>
          You show the tool where one job page says its title, company, location, posted date and description. It then reads every job of
          this link that way, <strong>without AI</strong>. The AI route stays available: &ldquo;Generate&rdquo; still writes instructions,
          and typing them by hand still works.
        </p>
        <div>
          <div className="font-semibold text-gray-900">Using it</div>
          <ol className="list-decimal pl-5">
            <li>Choose a sample page: one of this link&apos;s saved jobs, or another address on its own site.</li>
            <li>
              For each field take the suggestion that is right, pick the text from the page, or say it is not on the page. The description
              is the only required field. A value the page itself states (its JobPosting data) is offered first, because it survives a
              redesign of the page.
            </li>
            <li>
              For a long or messy description use a <em>description rule</em>: several boxes, things to leave out (cookie notices, share
              bars, sidebars), headings, where to stop, the format and a limit (60,000 characters unless you set another; plain fields are
              cut at 5,000).
            </li>
            <li>
              <em>Check on sample jobs</em>: the picks are tried on up to three saved jobs of this link, one after the other, with the real
              crawler. Checking opens no window and saves nothing.
            </li>
            <li>Read the values and the flags, then save. A blocking flag stops the save; warnings need your tick.</li>
          </ol>
        </div>
        <div>
          <div className="font-semibold text-gray-900">What saving does</div>
          <ul className="list-disc pl-5">
            <li>
              It replaces the job-detail instructions this link has now. Nothing is recrawled by saving: run &ldquo;Crawl job details
              now&rdquo; yourself.
            </li>
            <li>
              When a job&apos;s page is crawled, its description is saved, and its <strong>empty</strong> company, location and posted date
              are filled from the picks. A value a job already has is never replaced, and jobs you added by hand are never touched.
            </li>
            <li>
              The title is only checked, unless you tick &ldquo;Replace the job&apos;s title&rdquo;: then the page&apos;s title (cleaned
              like a listing title) replaces the saved one, every change is written in the run log, and a title over several lines is
              ignored.
            </li>
          </ul>
        </div>
        <div>
          <div className="font-semibold text-gray-900">What it needs</div>
          <ul className="list-disc pl-5">
            <li>The link must not need a login: for those the AI instructions are used.</li>
            <li>
              The page&apos;s own data (and a description read from it) needs the current Browser tool; with an older one the screen says
              so.
            </li>
            <li>
              Pages that fill their text in after load, or only after a click, may not be readable: the check on sample jobs shows it before
              you save. A site can change its design later; then check again.
            </li>
          </ul>
        </div>
      </div>
    </section>
  )
}
