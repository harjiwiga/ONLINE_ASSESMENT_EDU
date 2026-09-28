import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    watch: { usePolling: true, interval: 500 },
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
