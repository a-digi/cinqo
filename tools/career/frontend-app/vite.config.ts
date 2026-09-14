import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import cssInjectedByJsPlugin from 'vite-plugin-css-injected-by-js'

// Same build mechanism as browser's own step 15 (reused unchanged,
// see plan/ai/tools/career/step-05-react-tailwind-frontend.md) — one
// self-contained tools/career/frontend/bundle.js, the exact fixed
// path paths.go's frontendBundleFilename and the host's
// GET /{slug}/frontend-bundle route expect.
export default defineConfig({
  plugins: [react(), cssInjectedByJsPlugin()],
  // Required for the same reason browser's own step 15 needed it:
  // Vite library mode doesn't auto-replace process.env.NODE_ENV the
  // way app-mode builds do, and React's own bundled code checks it at
  // runtime.
  define: {
    'process.env.NODE_ENV': JSON.stringify('production'),
  },
  build: {
    lib: {
      entry: 'src/main.tsx',
      formats: ['iife'],
      name: 'CareerToolFrontend',
      fileName: () => 'bundle.js',
    },
    outDir: '../frontend',
    emptyOutDir: true,
  },
})
