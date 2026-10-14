import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// `npm run dev` serves the UI with hot reload and forwards /api and the
// manual (/help) to a deelook server started with `go run .` (default port
// 8090). Open /hmi or /sampling.
export default defineConfig({
  plugins: [svelte()],
  server: {
    proxy: { '/api': 'http://127.0.0.1:8090', '/help': 'http://127.0.0.1:8090' },
  },
})
