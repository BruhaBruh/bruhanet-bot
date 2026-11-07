import { fileURLToPath } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  build: { outDir: 'dist' },
  server: {
    // dev-вход: Telegram не принимает localhost в URL-кнопке, а *.localtest.me резолвится в 127.0.0.1 (ADMIN_BASE_URL в .env)
    allowedHosts: ['.localtest.me'],
    proxy: {
      '/api': 'http://localhost:8090',
      '/auth': 'http://localhost:8090',
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['src/test/setup.ts'],
  },
})
