import '@/assets/document-request-prototype.css';

import OpenVue from 'openvue/config';
import { createApp } from 'vue';

import DocumentRequestPrototype from '@/views/DocumentRequestPrototype.vue';

createApp(DocumentRequestPrototype).use(OpenVue, { unstyled: true }).mount('#app');
