import axios from 'axios';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import { usePublicLinkStore } from '@/stores/publicLink';
import type { FileItem, SecureLink, User } from '@/types/models';

describe('secure-link permissions', () => {
  beforeEach(() => {
    localStorage.clear();
    setActivePinia(createPinia());
    vi.restoreAllMocks();
    useAuthStore().user = { role: 'link' } as User;
  });

  it.each([
    ['read', true, false, false],
    ['view', true, false, false],
    ['write', false, true, false],
    ['readwrite', true, true, true],
    ['unknown', false, false, false],
  ])('honors %s access', (access, read, write, register) => {
    const link = usePublicLinkStore();
    link.link = {
      access_type: access,
      expires_at: new Date(Date.now() + 3600000).toISOString(),
    } as SecureLink;
    const files = useFileManagerStore();
    expect(files.canRead).toBe(read);
    expect(files.canWrite).toBe(write);
    expect(link.canRegister).toBe(register);
  });

  it('denies access before validation, during validation and after failure or expiry', () => {
    const link = usePublicLinkStore();
    const files = useFileManagerStore();
    expect(files.canWrite).toBe(false);
    expect(files.canRead).toBe(false);
    link.link = { access_type: 'readwrite', expires_at: '2000-01-01' } as SecureLink;
    expect(files.canWrite).toBe(false);
    link.link.expires_at = new Date(Date.now() + 3600000).toISOString();
    link.validating = true;
    expect(files.canWrite).toBe(false);
    link.validating = false;
    link.error = 'Link unavailable';
    expect(files.canWrite).toBe(false);
  });

  it('does not send mutations from a read-only session', async () => {
    const link = usePublicLinkStore();
    link.link = {
      access_type: 'read',
      expires_at: new Date(Date.now() + 3600000).toISOString(),
    } as SecureLink;
    const files = useFileManagerStore();
    const post = vi.spyOn(axios, 'post');
    const del = vi.spyOn(axios, 'delete');
    expect(await files.createFolder('New', { targetFolderId: 'root' })).toBeNull();
    expect((await files.uploadFiles([], false, undefined, { targetFolderId: 'root' })).ok).toBe(
      false,
    );
    await files.deleteItem({ id: 'file' } as FileItem);
    expect(post).not.toHaveBeenCalled();
    expect(del).not.toHaveBeenCalled();
  });

  it('does not apply a previous link restriction to signed-in client accounts', () => {
    useAuthStore().user = { role: 'client' } as User;
    expect(useFileManagerStore().canWrite).toBe(true);
    expect(useFileManagerStore().canRead).toBe(true);
  });
});
