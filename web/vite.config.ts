import { fileURLToPath } from 'node:url';

import { PrimeVueResolver } from '@primevue/auto-import-resolver';
import { sentryVitePlugin } from '@sentry/vite-plugin';
import tailwindcss from '@tailwindcss/vite';
import vue from '@vitejs/plugin-vue';
import AutoImport from 'unplugin-auto-import/vite';
import Components from 'unplugin-vue-components/vite';
import { defineConfig, loadEnv } from 'vite';
import vueDevTools from 'vite-plugin-vue-devtools';

function portFromEnv(value: string | undefined, fallback: number, name: string): number {
  const port = Number(value ?? fallback);
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(`${name} must be an integer between 1 and 65535`);
  }

  return port;
}

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), 'VITE_');
  const frontendPort = portFromEnv(env.VITE_FRONTEND_PORT, 5193, 'VITE_FRONTEND_PORT');
  const backendPort = portFromEnv(env.VITE_BACKEND_PORT, 8320, 'VITE_BACKEND_PORT');

  return {
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
      port: frontendPort,
      strictPort: true,
      hmr: {
        host: 'localhost',
        port: frontendPort,
        clientPort: frontendPort,
      },
      proxy: {
        '/api': {
          target: `http://localhost:${backendPort}`,
          changeOrigin: true,
          secure: false,
        },
      },
    },
  };
});
