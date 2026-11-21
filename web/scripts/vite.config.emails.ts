import { fileURLToPath } from 'node:url';

import vue from '@vitejs/plugin-vue';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [vue()],
  optimizeDeps: { noDiscovery: true },
  server: {
    fs: {
      allow: [
        fileURLToPath(new URL('..', import.meta.url)),
        fileURLToPath(new URL('../../shared', import.meta.url)),
      ],
    },
  },
});
