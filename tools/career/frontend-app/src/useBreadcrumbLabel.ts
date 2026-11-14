import { useEffect } from 'react'

// Puts the page's own words (a job's title, a company's name) in place of the fixed label of one item of its breadcrumb trail, once they are
// known (plan/ai/frontend/step-270-breadcrumb.md, step 7). Nothing is sent while `label` is missing, so the fixed word stays while the page
// loads. A host older than step 7 has no setBreadcrumbLabel: nothing happens and nothing breaks.
export function useBreadcrumbLabel(index: number, label: string | undefined): void {
  useEffect(() => {
    if (!label) return
    const bridge = window.__cinqoToolBridge as { setBreadcrumbLabel?: (index: number, label: string) => void }
    if (typeof bridge.setBreadcrumbLabel === 'function') bridge.setBreadcrumbLabel(index, label)
  }, [index, label])
}
