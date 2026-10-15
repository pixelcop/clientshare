import { defineConfig } from 'vitepress';

const embedded = process.env.DOCS_EMBEDDED === 'true';

export default defineConfig({
  title: 'ClientShare',
  description: 'Send files, collect documents, and manage your client portal with the ClientShare user guide.',
  lang: 'en-US',
  srcDir: './src',
  base: embedded ? '/docs/' : '/',
  outDir: embedded ? '../internal/static/guide/dist' : undefined,
  lastUpdated: process.env.DOCS_LAST_UPDATED !== 'false',
  // The shared app logo lives outside this package; use the guide's Vue runtime.
  vite: { resolve: { dedupe: ['vue'] } },
  themeConfig: {
    siteTitle: 'User Guide',
    nav: [
      { text: 'User Guide', link: '/getting-started/welcome' },
      { text: 'For Clients', link: '/for-clients/send-and-receive-documents' },
    ],
    search: { provider: 'local' },
    outline: [2, 3],
    sidebar: [
      {
        text: 'Getting started',
        items: [
          { text: 'Welcome to ClientShare', link: '/getting-started/welcome' },
          { text: 'Your first client exchange', link: '/getting-started/first-client-exchange' },
          { text: 'Sign in and navigation', link: '/getting-started/sign-in' },
          { text: 'Roles and access', link: '/getting-started/roles-and-access' },
        ],
      },
      {
        text: 'Clients and users', collapsed: false,
        items: [
          { text: 'Manage clients', link: '/clients/manage-clients' },
          { text: 'Invite and manage users', link: '/clients/manage-users' },
        ],
      },
      {
        text: 'Files and folders', collapsed: false,
        items: [
          { text: 'Organize folders', link: '/files/organize-folders' },
          { text: 'Upload files', link: '/files/upload' },
          { text: 'Preview and download', link: '/files/preview-and-download' },
          { text: 'Delete files and folders', link: '/files/delete' },
        ],
      },
      {
        text: 'Sharing and collecting documents', collapsed: false,
        items: [
          { text: 'Create a secure link', link: '/sharing/create-link' },
          { text: 'Permissions and expiry', link: '/sharing/permissions-and-expiry' },
          { text: 'Request documents', link: '/sharing/request-documents' },
          { text: 'Review and revoke links', link: '/sharing/manage-links' },
        ],
      },
      {
        text: 'For clients and recipients', collapsed: false,
        items: [
          { text: 'Accept an invitation', link: '/for-clients/accept-invitation' },
          { text: 'Open a secure link', link: '/for-clients/open-secure-link' },
          { text: 'Send and receive documents', link: '/for-clients/send-and-receive-documents' },
        ],
      },
      {
        text: 'Activity and notifications', collapsed: true,
        items: [
          { text: 'Review activity', link: '/activity/review-activity' },
          { text: 'Email notifications', link: '/activity/email-notifications' },
        ],
      },
      {
        text: 'Portal administration', collapsed: true,
        items: [
          { text: 'Customize branding', link: '/administration/branding' },
          { text: 'Portal settings', link: '/administration/settings' },
        ],
      },
      {
        text: 'Account security', collapsed: true,
        items: [
          { text: 'Reset your password', link: '/security/reset-password' },
          { text: 'Use passkeys', link: '/security/passkeys' },
        ],
      },
      {
        text: 'Help and reference', collapsed: true,
        items: [
          { text: 'Troubleshooting', link: '/help/troubleshooting' },
          { text: 'Frequently asked questions', link: '/help/faq' },
          { text: 'Glossary', link: '/help/glossary' },
        ],
      },
    ],
    socialLinks: [{ icon: 'github', link: 'https://github.com/pixelcop/clientshare' }],
    footer: { message: 'ClientShare user guide · Secure client document exchange' },
  },
});
