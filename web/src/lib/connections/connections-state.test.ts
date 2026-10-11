import { describe, expect, it, vi } from 'vitest';
import type { LiveConnection, LiveHandler } from '$lib/live/live-connection';
import type { LiveMessage } from '$lib/live/protocol';
import { ConnectionsState } from './connections-state.svelte';
import { HealthStatus, SERVICE_HEALTH_CHANGED, SERVICES, Service, updateFor } from './services';

const REFUSED = 'connection refused';

vi.mock('svelte-sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));

class FakeLive {
	readonly handlers = new Map<string, LiveHandler>();

	on(type: string, handler: LiveHandler) {
		this.handlers.set(type, handler);
		return () => this.handlers.delete(type);
	}

	onReset() {
		return () => {};
	}

	emit(type: string, data: unknown) {
		this.handlers.get(type)?.({ seq: 1, epoch: 'E', type, topic: '', data } as LiveMessage);
	}
}

describe('ConnectionsState', () => {
	it('follows ServiceHealthChanged on the live socket', () => {
		const live = new FakeLive();
		const state = new ConnectionsState(live as unknown as LiveConnection);

		live.emit(SERVICE_HEALTH_CHANGED, {
			service: Service.JELLYFIN,
			healthy: false,
			error: REFUSED
		});
		expect(state.health[Service.JELLYFIN].status).toBe(HealthStatus.UNHEALTHY);
		expect(state.health[Service.JELLYFIN].error).toBe(REFUSED);

		live.emit(SERVICE_HEALTH_CHANGED, { service: Service.JELLYFIN, healthy: true, error: '' });
		expect(state.health[Service.JELLYFIN].status).toBe(HealthStatus.HEALTHY);

		state.destroy();
		expect(live.handlers.size).toBe(0);
	});

	it('tracks unsaved edits per service', () => {
		const state = new ConnectionsState();
		const form = state.forms[Service.PROWLARR];
		form.load({ url: 'http://prowlarr:9696', apiKey: '••••••••' });
		expect(form.dirty).toBe(false);
		form.values.url = 'http://prowlarr:9697';
		expect(form.dirty).toBe(true);
		expect(state.forms[Service.TMDB].dirty).toBe(false);
		form.discard();
		expect(form.dirty).toBe(false);
	});
});

describe('updateFor', () => {
	it('names only the one service', () => {
		expect(updateFor(SERVICES[Service.OPENSUBTITLES], { username: 'u', password: 'p' })).toEqual({
			openSubtitles: { username: 'u', password: 'p' }
		});
	});
});
