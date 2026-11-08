import { fileURLToPath } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// «@» — исходники админки (токены дизайна и примитивы ui общие), «~» — этот пакет.
const adminSrc = fileURLToPath(new URL('../admin/src', import.meta.url))

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': adminSrc, '~': fileURLToPath(new URL('./src', import.meta.url)) },
    dedupe: ['react', 'react-dom'],
  },
  build: { outDir: 'dist' },
  server: {
    port: 5180, // не 5174: админский Vite при занятом 5173 уходит на 5174
    strictPort: true,
    // туннель для проверки в Telegram (cloudflared/ngrok/localtunnel): Vite по умолчанию отвергает чужой Host
    allowedHosts: ['.trycloudflare.com', '.ngrok-free.app', '.ngrok.app', '.ngrok.io', '.loca.lt', '.lhr.life', '.localhost.run', '.pinggy.link', '.pinggy.io', '.serveousercontent.com', '.localtest.me'],
    proxy: { '/m/api': 'http://localhost:8092' },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['src/test/setup.ts'],
  },
})
