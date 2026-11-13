import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { isReplayURLSafe, type ReplayPolicy, RuntimeReplay } from '../services/runtimeReplay';

describe('runtime replay', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });
  function setup(startOnFailure = false) {
    const replay = { start: vi.fn(), stop: vi.fn().mockResolvedValue(undefined) };
    const policy: ReplayPolicy = {
      enabled: true,
      start_on_failure: startOnFailure,
      expires_at: new Date(Date.now() + 60_000).toISOString(),
    };
    const load = vi.fn().mockResolvedValue(policy);
    return { replay, policy, load, controller: new RuntimeReplay(replay, load) };
  }

  it('does not record credential-bearing URLs before bootstrap consumes them', async () => {
    [
      '/accept-invite?token=private',
      '/login#handoff=private',
      '/reset-password?code=private',
      '/links/private',
      '/login?redirect=/accept-invite?token=private',
    ].forEach((url) => expect(isReplayURLSafe(url)).toBe(false));
    expect(isReplayURLSafe('/login?redirect=/clients')).toBe(true);
    const { replay, load } = setup();
    const safe = vi.fn().mockReturnValue(false);
    const controller = new RuntimeReplay(replay, load, safe);
    await controller.refresh();
    expect(replay.start).not.toHaveBeenCalled();
    safe.mockReturnValue(true);
    await controller.refresh();
    expect(replay.start).toHaveBeenCalledOnce();
  });

  it('starts only when authorized, remains idempotent and revokes without flushing', async () => {
    const { controller, replay, load } = setup();
    await controller.refresh();
    await controller.refresh();
    expect(replay.start).toHaveBeenCalledTimes(1);
    load.mockResolvedValue({ enabled: false, start_on_failure: false });
    await controller.refresh();
    expect(replay.stop).toHaveBeenCalledExactlyOnceWith({ flush: false });
  });

  it('waits for 4xx/5xx and captures an initial auth failure racing config', async () => {
    const { controller, replay } = setup(true);
    controller.failure(401);
    await controller.refresh();
    expect(replay.start).toHaveBeenCalledTimes(1);
    await controller.suspend();
    await controller.refresh();
    controller.failure(302);
    await vi.advanceTimersByTimeAsync(0);
    expect(replay.start).toHaveBeenCalledTimes(1);
    controller.failure(500);
    await vi.advanceTimersByTimeAsync(0);
    expect(replay.start).toHaveBeenCalledTimes(2);
  });

  it('fails closed on config failure and rejects stale overlapping responses', async () => {
    const { controller, replay, load } = setup();
    let resolve!: (value: ReplayPolicy) => void;
    load.mockImplementationOnce(
      () =>
        new Promise<ReplayPolicy>((done) => {
          resolve = done;
        }),
    );
    const old = controller.refresh();
    load.mockResolvedValueOnce({ enabled: false, start_on_failure: false });
    await controller.refresh();
    resolve({
      enabled: true,
      start_on_failure: false,
      expires_at: new Date(Date.now() + 60_000).toISOString(),
    });
    await old;
    expect(replay.start).not.toHaveBeenCalled();
    await controller.refresh();
    load.mockRejectedValue(new Error('offline'));
    await controller.refresh();
    expect(replay.stop).toHaveBeenCalledWith({ flush: false });
  });

  it('stops at expiry even if refreshing configuration is offline', async () => {
    const { controller, replay, load } = setup();
    await controller.refresh();
    load.mockRejectedValue(new Error('offline'));
    await vi.advanceTimersByTimeAsync(60_001);
    expect(replay.stop).toHaveBeenCalledWith({ flush: false });
    controller.failure(401);
    await vi.advanceTimersByTimeAsync(0);
    expect(replay.start).toHaveBeenCalledTimes(1);
  });

  it('remembers failures racing a later policy refresh, but not old unmatched failures', async () => {
    const { controller, replay, load, policy } = setup(true);
    load.mockResolvedValueOnce({ enabled: false, start_on_failure: false });
    await controller.refresh();
    controller.failure(401);
    let resolve!: (value: ReplayPolicy) => void;
    load.mockImplementationOnce(
      () =>
        new Promise<ReplayPolicy>((done) => {
          resolve = done;
        }),
    );
    const refreshing = controller.refresh();
    controller.failure(500);
    resolve(policy);
    await refreshing;
    expect(replay.start).toHaveBeenCalledTimes(1);
  });

  it('restarts an authorized recording after the SDK ends its own session', async () => {
    const { replay, load } = setup();
    const active = vi.fn().mockReturnValue(false);
    const controller = new RuntimeReplay({ ...replay, active }, load);
    await controller.refresh();
    active.mockReturnValue(true);
    await controller.refresh();
    expect(replay.start).toHaveBeenCalledTimes(1);
    active.mockReturnValue(false);
    await controller.refresh();
    expect(replay.start).toHaveBeenCalledTimes(2);
  });

  it.each([undefined, 'bad', '2000-01-01T00:00:00Z'])(
    'never starts with invalid expiry %s',
    async (expiresAt) => {
      const { controller, replay, load } = setup();
      load.mockResolvedValue({ enabled: true, start_on_failure: false, expires_at: expiresAt });
      await controller.refresh();
      expect(replay.start).not.toHaveBeenCalled();
    },
  );
});
