import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { plainTextSelectors, render } from '@vue-email/render';

import PasswordResetEmail from '../emails/password-reset-email.vue';
import SecureLinkEmail from '../emails/secure-link-email.vue';
import UploadNotificationEmail from '../emails/upload-notification-email.vue';
import UserInviteEmail from '../emails/user-invite-email.vue';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(scriptDir, '..');
const outputDir = resolve(webRoot, '../internal/services/email/generated');

async function write(file: string, content: string) {
  const filePath = resolve(outputDir, file);
  await mkdir(dirname(filePath), { recursive: true });
  await writeFile(filePath, content);
}

async function renderHTML(file: string, tpl: any, data: Record<string, string>) {
  await write(file, await render(tpl, data, { pretty: true }));
}

async function renderText(file: string, tpl: any, data: Record<string, string>) {
  await write(
    file,
    await render(tpl, data, {
      plainText: true,
      htmlToTextOptions: {
        selectors: [
          ...plainTextSelectors,
          { selector: 'h1', options: { uppercase: false } },
          { selector: 'div', options: { uppercase: false } },
        ],
      },
    }),
  );
}

async function renderEmail(file: string, tpl: any, data: Record<string, string>) {
  await renderHTML(`${file}.html`, tpl, data);
  await renderText(`${file}.txt`, tpl, data);
}

await mkdir(outputDir, { recursive: true });

await renderEmail('password_reset', PasswordResetEmail, {
  displayName: '{{.DisplayName}}',
  resetLink: '{{.ResetLink}}',
  expiry: '{{.Expiry}}',
  productName: '{{.ProductName}}',
});

await renderEmail('upload_notification', UploadNotificationEmail, {
  clientName: '{{.ClientName}}',
  fileName: '',
  productName: '{{.ProductName}}',
});

await renderEmail('secure_link', SecureLinkEmail, {
  clientName: '{{.ClientName}}',
  accessType: '{{.AccessType}}',
  linkUrl: '{{.LinkURL}}',
  expiry: '{{.Expiry}}',
  productName: '{{.ProductName}}',
});

await renderEmail('user_invite', UserInviteEmail, {
  displayName: '{{.DisplayName}}',
  inviteUrl: '{{.InviteURL}}',
  expiry: '{{.Expiry}}',
  productName: '{{.ProductName}}',
  welcomeText: '{{.WelcomeText}}',
});

console.log('Email templates generated in internal/services/email/generated');
