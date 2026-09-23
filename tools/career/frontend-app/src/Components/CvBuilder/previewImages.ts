// Every template folder under Template/<id>/ may have its own
// preview.png — a real screenshot of that design rendered with sample
// data, so the template picker (PersonaAndTemplateStep.tsx) can show
// the user what each one actually looks like instead of just a name
// and a one-line description.
//
// `?inline` forces Vite to base64-inline the PNG directly into
// bundle.js regardless of its size, rather than emitting it as a
// separate file under an /assets/ URL — this frontend's own
// vite.config.ts is deliberately built around "exactly one servable
// output file" (cssInjectedByJsPlugin does the same for CSS), and the
// host has no route that would ever serve a second static file
// alongside bundle.js. import.meta.glob (not one import statement per
// template) means adding or removing a template's own preview.png
// later never needs a matching code change here.
// `?inline` isn't one of Vite's own well-known query types (raw/url/
// worker), so its own type inference can't tell this resolves to a
// string on its own — the explicit <string> generic is what makes that
// exact.
const modules = import.meta.glob<string>('./Template/*/preview.png', { eager: true, query: '?inline', import: 'default' })

const previewImagePattern = /^\.\/Template\/([^/]+)\/preview\.png$/

export const previewImages: Record<string, string> = Object.fromEntries(
  Object.entries(modules).flatMap(([path, dataUri]) => {
    const match = previewImagePattern.exec(path)
    return match ? [[match[1], dataUri]] : []
  }),
)
