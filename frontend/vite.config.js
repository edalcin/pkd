import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  build: {
    // Output directly into the Go embed directory
    outDir: '../internal/server/web/dist',
    emptyOutDir: true,
    minify: 'esbuild',
    rollupOptions: {
      output: {
        // Split large libraries into separate chunks for better caching
        manualChunks: {
          tiptap: [
            '@tiptap/core',
            '@tiptap/starter-kit',
            '@tiptap/extension-image',
            '@tiptap/suggestion',
          ],
          d3: ['d3-force', 'd3-selection', 'd3-zoom', 'd3-drag'],
        },
      },
    },
    // Inline small assets as base64 to reduce HTTP round-trips
    assetsInlineLimit: 4096,
  },
})
