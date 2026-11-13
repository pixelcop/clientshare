import * as Sentry from '@sentry/vue';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { replayPrivacyOptions } from '@/services/replayPrivacy';

describe('replay privacy', () => {
  afterEach(async () => {
    await Sentry.getReplay()?.stop({ flush: false });
    await Sentry.close();
    vi.unstubAllGlobals();
    document.body.innerHTML = '';
    document.title = '';
  });

  it('records static UI but scrubs private text, inputs, attributes and document content', async () => {
    vi.stubGlobal('__SENTRY_BROWSER_BUNDLE__', true);
    document.title = 'Private portal branding';
    document.body.innerHTML = `
      <nav>Clients</nav><button>Share workspace</button>
      <span data-sentry-mask>Private Client</span>
      <span data-sentry-mask>Private User private@example.test</span>
      <span data-sentry-mask>private-report.pdf 12 MB</span>
      <input value="private-input@example.test">
      <a href="/files/file-id" title="Private title" download="private.pdf">Download</a>
      <a href="https://private.example.test/links/private-link-secret">Open recipient view</a>
      <div role="alert">Failed to send to private-error@example.test</div>
      <div class="p-toast">Uploaded private-notification.pdf</div>
      <div data-sentry-block><pre>Private document content</pre></div>
      <img src="https://private.example.test/image.png" alt="Private image">
    `;
    const recordings: string[] = [];
    Sentry.init({
      dsn: 'https://public@example.invalid/1',
      defaultIntegrations: false,
      replaysSessionSampleRate: 0,
      replaysOnErrorSampleRate: 0,
      integrations: [
        Sentry.replayIntegration({
          ...replayPrivacyOptions,
          useCompression: false,
          minReplayDuration: 0,
        }),
      ],
      // Inspect the actual SDK output without sending anything to Sentry.
      transport: () => ({
        send: async (envelope) => {
          envelope[1].forEach(([headers, payload]) => {
            if (headers.type === 'replay_recording') {
              recordings.push(
                typeof payload === 'string'
                  ? payload
                  : new TextDecoder().decode(payload as Uint8Array),
              );
            }
          });
          return { statusCode: 200 };
        },
        flush: async () => true,
      }),
    });
    const replay = Sentry.getReplay()!;
    replay.start();
    await replay.flush();
    expect(recordings.length).toBeGreaterThan(0);
    const recording = recordings.join('');
    expect(recording).toContain('Clients');
    expect(recording).toContain('Share workspace');
    expect(recording).toContain('Download');
    expect(recording).not.toContain('Private portal branding');
    [
      'Private Client',
      'Private User',
      'private@example.test',
      'private-report.pdf',
      '12 MB',
      'private-input@example.test',
      'private.example.test',
      'Private title',
      'private.pdf',
      'private-error@example.test',
      'private-notification.pdf',
      'Private document content',
      'Private image',
    ].forEach((sensitive) => {
      expect(recording).not.toContain(sensitive);
    });
    document.querySelector('[data-sentry-mask]')!.textContent = 'Updated Private Client';
    document.body.insertAdjacentHTML(
      'beforeend',
      '<p>Upload complete</p><span data-sentry-mask>New Private User</span>',
    );
    await vi.waitFor(async () => {
      await replay.flush();
      expect(recordings.join('')).toContain('Upload complete');
    });
    expect(recordings.join('')).not.toContain('Updated Private Client');
    expect(recordings.join('')).not.toContain('New Private User');
  });
});
