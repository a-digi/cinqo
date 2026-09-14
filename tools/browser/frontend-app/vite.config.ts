import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import cssInjectedByJsPlugin from 'vite-plugin-css-injected-by-js'

// Builds this tool's own frontend as ONE self-contained file,
// tools/browser/frontend/bundle.js — the exact fixed path
// paths.go's frontendBundleFilename expects on disk, and the only
// path the host's GET /{slug}/frontend-bundle route ever serves.
// library mode + the IIFE format produce a single script (no
// index.html shell, no code-split chunks); vite-plugin-css-injected-by-js
// inlines the compiled Tailwind CSS into that same file via a
// runtime <style> tag instead of emitting a second, unservable
// bundle.css. Verified directly with a throwaway PoC before writing
// this config — see
// plan/ai/tools/browser/step-15-react-tailwind-frontend-rewrite.md.
export default defineConfig({
  plugins: [react(), cssInjectedByJsPlugin()],
  // React/ReactDOM's own bundled code checks process.env.NODE_ENV at
  // runtime (dev-mode-only branches) — Vite's app-mode build replaces
  // that automatically, but library mode does not, so it survives
  // into the output unreplaced and throws "process is not defined" in
  // a real browser (caught live, not by review — see this step's own
  // "Implemented and verified" section). Defining it explicitly here
  // fixes it for library mode.
  define: {
    'process.env.NODE_ENV': JSON.stringify('production'),
  },
  build: {
    lib: {
      entry: 'src/main.tsx',
      formats: ['iife'],
      name: 'BrowserToolFrontend',
      fileName: () => 'bundle.js',
    },
    // Writes straight into the install-tree's own frontend/ directory
    // — no separate copy step in the Makefile needed. This directory
    // holds nothing but the generated bundle.js; safe to empty on
    // every build.
    outDir: '../frontend',
    emptyOutDir: true,
  },
})
