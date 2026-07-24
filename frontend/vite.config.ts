import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // Backend runs on :8080 by default (see backend/internal/config).
      // `npm run dev` proxies API/WS calls there so the UI can be developed
      // without rebuilding the Go binary on every change.
      '/api': {
        target: 'http://localhost:8080',
        ws: true,
        changeOrigin: true,
      },
    },
  },
})
