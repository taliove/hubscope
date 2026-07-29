import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// Build output goes to web/dist so the Go binary can go:embed it.
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // Split the two big dependencies into stable vendor chunks. Vite emits
        // content-hashed filenames, so vendor chunks keep their hash across
        // business-code releases and stay warm in immutable caches; only the
        // app chunk changes. Keep this list conservative (no vue runtime
        // splitting) to avoid module-initialization-order surprises.
        manualChunks: {
          echarts: ['echarts'],
          'element-plus': ['element-plus', '@element-plus/icons-vue'],
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      // Forward API calls to the Go backend during local development.
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
