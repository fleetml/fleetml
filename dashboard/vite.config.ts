import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// The dashboard talks to the API on its own origin, so the session cookie is
// first-party: the dev server and `vite preview` proxy to the API.
const proxy = {
  '/api': { target: process.env.FLEETML_API || 'http://localhost:18080', changeOrigin: true },
  '/healthz': { target: process.env.FLEETML_API || 'http://localhost:18080', changeOrigin: true },
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: { port: 5173, proxy },
  preview: { port: 4173, proxy },
})
