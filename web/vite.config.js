import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// The build lands where Go embeds it, so `croft serve` carries the interface.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: '../internal/server/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: { '/api': 'http://localhost:8080' },
  },
})
