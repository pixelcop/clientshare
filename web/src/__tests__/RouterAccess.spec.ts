import { beforeEach, describe, expect, it, vi } from 'vitest';

import router from '@/router';

const auth = vi.hoisted(() => ({
  user: null as { role: string } | null,
  fetchMe: vi.fn(),
}));

vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }));
vi.mock('@/views/ClientFiles.vue', () => ({ default: {} }));
vi.mock('@/views/AcceptInviteView.vue', () => ({ default: {} }));
vi.mock('@/views/ActivityFeed.vue', () => ({ default: {} }));
vi.mock('@/views/ClientsList.vue', () => ({ default: {} }));
vi.mock('@/views/FileView.vue', () => ({ default: {} }));
vi.mock('@/views/ForgotPasswordView.vue', () => ({ default: {} }));
vi.mock('@/views/LoginView.vue', () => ({ default: {} }));
vi.mock('@/views/ResetPasswordView.vue', () => ({ default: {} }));
vi.mock('@/views/SecureLinks.vue', () => ({ default: {} }));
vi.mock('@/views/SecurityView.vue', () => ({ default: {} }));
vi.mock('@/views/SettingsView.vue', () => ({ default: {} }));
vi.mock('@/views/UsersList.vue', () => ({ default: {} }));

beforeEach(async () => {
  auth.user = null;
  await router.replace('/login');
});

describe('folder navigation', () => {
  it.each(['admin', 'manager', 'client', 'link'])(
    'lets %s sessions reach a client folder and its root',
    async (role) => {
      auth.user = { role };
      await router.push('/folder/example-folder');
      expect(router.currentRoute.value.name).toBe('FolderView');
      expect(router.currentRoute.value.params.folderId).toBe('example-folder');

      await router.push('/folders/another-folder');
      expect(router.currentRoute.value.params.folderId).toBe('another-folder');

      await router.push('/folder');
      expect(router.currentRoute.value.name).toBe('FolderRootView');
    },
  );

  it.each(['client', 'link'])(
    'routes %s file entry points without redirect loops',
    async (role) => {
      auth.user = { role };
      await router.push('/files');
      expect(router.currentRoute.value.name).toBe('FolderRootView');
    },
  );

  it.each(['admin', 'manager'])(
    'keeps the %s /files shortcut directed to Clients',
    async (role) => {
      auth.user = { role };
      await router.push('/files');
      expect(router.currentRoute.value.name).toBe('Clients');
    },
  );

  it('requires sign-in for a direct folder link', async () => {
    await router.push('/folder/example-folder');
    expect(router.currentRoute.value.name).toBe('Login');
    expect(router.currentRoute.value.query.redirect).toBe('/folder/example-folder');
  });

  it.each(['manager', 'client', 'link'])('keeps Settings restricted for %s', async (role) => {
    auth.user = { role };
    await router.push('/settings');
    expect(router.currentRoute.value.name).toBe(role === 'manager' ? 'Clients' : 'FolderRootView');
  });
});
