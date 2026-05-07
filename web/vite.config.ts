import { fileURLToPath } from 'node:url';

import { PrimeVueResolver } from '@primevue/auto-import-resolver';
import { sentryVitePlugin } from '@sentry/vite-plugin';
import tailwindcss from '@tailwindcss/vite';
import vue from '@vitejs/plugin-vue';
import AutoImport from 'unplugin-auto-import/vite';
import Components from 'unplugin-vue-components/vite';
import { defineConfig } from 'vite';
import vueDevTools from 'vite-plugin-vue-devtools';

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    vueDevTools(),
    tailwindcss(),

    Components({
      resolvers: [PrimeVueResolver()],
    }),

    AutoImport({
      include: [
        /\.[tj]sx?$/, // .ts, .tsx, .js, .jsx
        /\.vue$/,
        /\.vue\?vue/, // .vue
        /\.vue\.[tj]sx?\?vue/, // .vue (vue-loader with experimentalInlineMatchResource enabled)
        /\.md$/, // .md
      ],
      imports: ['vue', 'vue-router'],
      dirsScanOptions: {
        filePatterns: ['*.ts'],
        fileFilter: (file) => file.endsWith('.ts'),
        types: true,
      },
      dirs: ['./composables/**'],
      dts: './auto-imports.d.ts',
      dtsMode: 'overwrite',
      dtsPreserveExts: false,
      viteOptimizeDeps: true,
      injectAtEnd: true,
      eslintrc: {
        enabled: true,
        filepath: './.eslintrc-auto-import.json',
        globalsPropValue: true,
      },
    }),
    sentryVitePlugin({
      org: 'pixelcop-research',
      project: 'clientshare-vue',
      telemetry: false,
    }),
  ],

  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },

  css: {
    preprocessorOptions: {
      scss: {
        api: 'modern-compiler',
      },
    },
  },

  build: {
    outDir: '../internal/static/dist',
    emptyOutDir: true,
    sourcemap: true,
  },

  server: {
    host: '0.0.0.0',
    port: 5173,
    strictPort: true,
    hmr: {
      host: 'localhost',
      port: 5173,
      clientPort: 5173,
    },
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        secure: false,
      },
    },
  },
});
