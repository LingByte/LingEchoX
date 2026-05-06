import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  base: '/',
  resolve: {
    dedupe: ['react', 'react-dom'],
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 3000,
    open: true,
    host: true, // 允许外部访问
    hmr: {
      port: 3001, // 使用不同的端口用于HMR
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false, // 生产环境关闭sourcemap提升性能
    minify: 'terser',
    terserOptions: {
      compress: {
        drop_console: true, // 移除console
        drop_debugger: true, // 移除debugger
      },
    },
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['react', 'react-dom'],
          arco: ['@arco-design/web-react'],
          router: ['react-router-dom'],
          utils: ['zustand', 'clsx', 'tailwind-merge'],
        },
      },
    },
    // 启用gzip压缩
    reportCompressedSize: true,
    // 设置chunk大小警告限制
    chunkSizeWarningLimit: 1000,
  },
  // 优化依赖预构建
  optimizeDeps: {
    include: [
      'react',
      'react-dom',
      '@arco-design/web-react',
      'react-router-dom',
      'zustand',
    ],
  },
})
