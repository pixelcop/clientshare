import '@/assets/main.css';
import '@/assets/tailwind.css';
import '@/assets/styles.scss';

import Aura from '@openvue/themes/aura';
import * as Sentry from '@sentry/vue';
import OpenVue from 'openvue/config';
import ConfirmationService from 'openvue/confirmationservice';
import ToastService from 'openvue/toastservice';
import { createPinia } from 'pinia';

import App from './App.vue';
import router from './router';
import {
  isReplayURLSafe,
  startRuntimeReplay,
  suspendRuntimeReplay,
} from './services/runtimeReplay';

const app = createApp(App);
const pinia = createPinia();

const sentryDSN = import.meta.env.VITE_SENTRY_DSN;
if (sentryDSN) {
  Sentry.init({
    app,
    dsn: sentryDSN,
    replaysSessionSampleRate: 0,
    replaysOnErrorSampleRate: 0,
    integrations: [
      Sentry.replayIntegration({
        stickySession: false,
        maskAllText: true,
        maskAllInputs: true,
        maskAttributes: ['title', 'placeholder', 'aria-label', 'href', 'src', 'srcset', 'action'],
        blockAllMedia: true,
        block: ['iframe', 'canvas'],
        networkCaptureBodies: false,
        networkDetailAllowUrls: [],
        // Console payloads and URLs may contain document names or invite tokens.
        // Keep the replay visual-only rather than opt into unsafe recording data.
        beforeAddRecordingEvent: () => null,
      }),
    ],
  });
  pinia.use(Sentry.createSentryPiniaPlugin());
  Sentry.addEventProcessor((event) => {
    if (event.type === 'replay_event' && 'urls' in event && Array.isArray(event.urls)) {
      return {
        ...event,
        urls: event.urls.map((value: unknown) => {
          try {
            if (typeof value !== 'string') {return '[Filtered]';}
            const url = new URL(value);
            return url.origin + url.pathname;
          } catch {
            return '[Filtered]';
          }
        }),
      };
    }
    return event;
  });
}
startRuntimeReplay();
router.beforeEach(async (to) => {
  if (!isReplayURLSafe(to.fullPath)) {
    await suspendRuntimeReplay();
  }
});

app.use(pinia);
app.use(router);
app.use(OpenVue, {
  unstyled: false,
  theme: {
    preset: Aura,
    options: {
      darkModeSelector: '.app-dark',
      cssLayer: {
        name: 'primevue',
        order: 'theme, base, primevue',
      },
    },
  },
});
app.use(ToastService);
app.use(ConfirmationService);

app.mount('#app');
