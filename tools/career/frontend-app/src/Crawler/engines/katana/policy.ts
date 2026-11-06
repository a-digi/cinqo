// Whether, under Katana, the app starts the AI for a link's job details by itself once its jobs are crawled (the chain of plan 192, D1/D8):
// the crawl's "jobs crawled" event raises the request, and an open Platforms page starts the AI run with the Katana job-detail recipe, exactly
// as Rod's chain does. It was off while the first real runs were watched (plan 192, step 10) and is ON since the user asked for the chain to
// run by itself. Turning it off again leaves the Generate button on the Job details tile as the only start.
export const AUTO_JOB_DETAILS = true
