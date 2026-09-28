import { defineConfig } from 'vite'

// Separate build for the mobile WebView editor page (editor.html), kept out of
// vite.config.js on purpose: it needs base:'./' so the built page works from
// file:///android_asset/editor/ in the Android app, which would break the PWA's
// absolute-path index.html/service-worker-less asset references if shared with
// the main build. Run: npm run build:editor. Vendored into pkdMobile by its own
// scripts/update-editor-bundle (ADR pkdMobile 0002) — never embedded into the Go
// binary, so outDir stays out of internal/server/web/dist.
export default defineConfig({
  base: './',
  publicDir: false, // frontend/public/ is PWA-only (manifest, icons, service worker)
  build: {
    outDir: 'dist-editor',
    emptyOutDir: true,
    minify: 'esbuild',
    rollupOptions: {
      input: 'editor.html',
    },
  },
})
