import type { replayIntegration } from '@sentry/vue';

// Static UI stays readable. Data-bearing DOM must opt into data-sentry-mask;
// file content must use data-sentry-block (including non-media text previews).
// Keep these rules on shared components so every screen inherits protection.
export const replayPrivacyOptions = {
  stickySession: false,
  maskAllText: false,
  maskAllInputs: true,
  maskAttributes: [
    'title',
    'placeholder',
    'aria-label',
    'aria-description',
    'alt',
    'href',
    'src',
    'srcset',
    'action',
    'download',
  ],
  // Notifications can interpolate filenames, recipients, and server errors.
  mask: ['title', '.p-toast', '.p-message', '[role="alert"]'],
  blockAllMedia: true,
  // rrweb normalizes URL attributes before maskAttributes runs. Block links
  // that can contain bearer credentials/contact data instead of trusting href
  // masking. Ordinary route links and their UI labels remain visible.
  block: [
    'iframe',
    'canvas',
    'a[href*="/links/"]',
    'a[href*="/api/link/"]',
    'a[href*="?"]',
    'a[href*="#"]',
    'a[href^="mailto:"]',
    'a[href^="tel:"]',
  ],
  networkCaptureBodies: false,
  networkDetailAllowUrls: [],
  // Console/network payloads may include credentials or document metadata.
  beforeAddRecordingEvent: () => null,
} satisfies NonNullable<Parameters<typeof replayIntegration>[0]>;

// Overlay content is teleported outside its field's masked ancestor.
export const replaySensitiveSelectPT = {
  root: { 'data-sentry-mask': true },
  overlay: { 'data-sentry-mask': true },
};

export const replaySensitiveTitlePT = {
  title: { 'data-sentry-mask': true },
};
