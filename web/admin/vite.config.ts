import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [vue()],
  build: { outDir: 'dist' },
  server: {
    proxy: {
      '/api': 'http://localhost:8090',
      '/auth': 'http://localhost:8090',
    },
  },
})
