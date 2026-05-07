import { fileURLToPath } from 'node:url';

import AutoImport from 'unplugin-auto-import/vite';
import { configDefaults,defineConfig, mergeConfig } from 'vitest/config';

import viteConfig from './vite.config';

export default mergeConfig(
  viteConfig,
  defineConfig({
    plugins: [
      AutoImport({
        imports: ['vue', 'vue-router', 'vitest'],
        dts: './auto-imports.vitest.d.ts',
        dtsMode: 'overwrite',
      }),
    ],

    test: {
      environment: 'jsdom',
      exclude: [...configDefaults.exclude, 'e2e/**'],
      root: fileURLToPath(new URL('./', import.meta.url)),
    },
  }),
);
