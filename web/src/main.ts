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

const app = createApp(App);
const pinia = createPinia();

const sentryDSN = import.meta.env.VITE_SENTRY_DSN;
if (sentryDSN) {
  Sentry.init({
    app,
    dsn: sentryDSN,
    sendDefaultPii: true,
  });
  pinia.use(Sentry.createSentryPiniaPlugin());
}

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
