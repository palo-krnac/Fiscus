import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    // pri vývoji beží Go API zvlášť: fiskus -demo
    proxy: { '/api': 'http://127.0.0.1:8080' },
  },
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 2000,
  },
  test: {
    environment: 'jsdom',
  },
})
