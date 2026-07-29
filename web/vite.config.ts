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
        // app chunk changes. Function form (audit 2026-07-29): the object form
        // ('element-plus': ['element-plus']) force-included the whole package
        // and defeated tree-shaking of the on-demand import in main.ts; the
        // function only sees modules that survived shaking.
        manualChunks(id) {
          if (id.includes('node_modules/echarts')) return 'echarts'
          if (id.includes('node_modules/element-plus') || id.includes('node_modules/@element-plus'))
            return 'element-plus'
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
