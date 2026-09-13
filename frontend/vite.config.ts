import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  // Unprefixed on purpose — never VITE_-prefixed. A VITE_ prefix gets
  // inlined into the client bundle; this value must only ever be known
  // to the dev-server proxy config, never shipped to the browser. The
  // frontend calls its own origin (/api, /auth) and never learns where
  // the real backend lives. See plan/ai/backend/auth-integration
  // decisions (coco-aim/plan/auth-integration.md §3).
  const apiUrl = env.API_URL || 'http://localhost:7026'

  return {
    plugins: [react()],
    server: {
      // Pinned — config/config.json's frontend_callback_url is
      // hardcoded to this port. Vite's default (5173, which we're
      // already using) would silently break the OAuth redirect back
      // into the app if it ever drifted from what's configured there.
      port: 5173,
      strictPort: true,
      proxy: {
        '/api': { target: apiUrl, changeOrigin: true },
        // Everything under /auth belongs to the BACKEND's own API
        // (currently GET /auth/callback). The frontend's own PKCE
        // callback PAGE deliberately lives at a different path
        // (/login/callback, not /auth/callback) — colliding here would
        // let this proxy rule swallow the SPA route and serve the
        // backend's raw JSON instead.
        //
        // The key is an ANCHORED regex (`^/auth(/|$)`), NOT a bare
        // `/auth` prefix: a prefix match would also swallow sibling SPA
        // routes that happen to start with "/auth" (e.g. a future
        // `/authorization` page), proxying them to the backend (which
        // 404s). Match only `/auth` exactly or `/auth/...`.
        '^/auth(/|$)': { target: apiUrl, changeOrigin: true },
      },
    },
  }
})
