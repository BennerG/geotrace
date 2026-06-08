import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/ingest': 'http://localhost:8090',
      '/events': 'http://localhost:8090',
      '/stats': 'http://localhost:8090',
      '/health': 'http://localhost:8090',
      '/ws': {
        target: 'ws://localhost:8090',
        ws: true,
      },
    },
  },
  build: {
    outDir: '../cmd/server/dist',
    emptyOutDir: true,
  },
})
