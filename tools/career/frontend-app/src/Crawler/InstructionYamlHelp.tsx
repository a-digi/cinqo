// What the YAML of an instruction needs, in the right column of the Rod instruction modals (plan 246). Every statement is a check the Career backend makes
// when the instructions are saved (portal/portals.go: validateCrawlInstructions and validateJobDetailCrawlInstructions).
export function InstructionYamlHelp({ kind }: { kind: 'list' | 'detail' }) {
  return (
    <section className="text-xs leading-relaxed text-gray-700" data-testid={`instruction-yaml-help-${kind}`}>
      <h3 className="text-sm font-semibold text-gray-900">
        {kind === 'list' ? 'Writing the job list instructions' : 'Writing the job detail instructions'}
      </h3>
      {kind === 'list' ? (
        <div className="mt-3 space-y-3">
          <div>
            <div className="font-semibold text-gray-900">Fields</div>
            <p>
              <span className="font-mono">fields</span> needs at least one entry, and every entry a <span className="font-mono">label</span>{' '}
              and a <span className="font-mono">selector</span> (optionally an <span className="font-mono">attribute</span> to read, or{' '}
              <span className="font-mono">multiple: true</span>).
            </p>
          </div>
          <div>
            <div className="font-semibold text-gray-900">What a job needs</div>
            <p>
              The fields must produce a <span className="font-mono">title</span> and a <span className="font-mono">url</span>: label a field
              with that name, or rename one with <span className="font-mono">mapping: &#123; yourLabel: title &#125;</span>. A job without
              both is skipped.
            </p>
          </div>
          <div>
            <div className="font-semibold text-gray-900">Pagination</div>
            <p>
              <span className="font-mono">pagination</span> is required, with a <span className="font-mono">nextSelector</span> and{' '}
              <span className="font-mono">maxPages</span> above 0.
            </p>
          </div>
          <div>
            <div className="font-semibold text-gray-900">Optional</div>
            <p>
              <span className="font-mono">container</span>: a selector for the element that wraps one job, so every field is read inside it.
            </p>
          </div>
          <p>Saving only checks the shape; whether the selectors find the jobs is shown by a crawl.</p>
        </div>
      ) : (
        <div className="mt-3 space-y-3">
          <div>
            <div className="font-semibold text-gray-900">Fields</div>
            <p>
              Every entry needs a <span className="font-mono">label</span> and a <span className="font-mono">selector</span> (optionally an{' '}
              <span className="font-mono">attribute</span> or <span className="font-mono">multiple: true</span>), or a{' '}
              <span className="font-mono">source</span> and <span className="font-mono">path</span> to read the page&apos;s own data instead
              of an element, never both.
            </p>
          </div>
          <div>
            <div className="font-semibold text-gray-900">What it needs</div>
            <p>
              A <span className="font-mono">description</span>: a field labelled that way, or a{' '}
              <span className="font-mono">descriptionRule</span>. There is no pagination and no container for a single job page.
            </p>
          </div>
          <div>
            <div className="font-semibold text-gray-900">Optional</div>
            <p>
              <span className="font-mono">replaceTitle: true</span> lets the page&apos;s title replace the saved one; it needs a field
              labelled <span className="font-mono">title</span>.
            </p>
          </div>
          <p>Saving only checks the shape; the check on sample jobs shows whether the selectors find the text.</p>
        </div>
      )}
    </section>
  )
}
