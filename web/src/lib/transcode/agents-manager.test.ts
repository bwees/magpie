import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { AgentResponse, UpdateAgentRequest } from '$lib/sdk';
import type { LiveHandler, LiveConnection, ResetListener } from '$lib/live/live-connection';
import type { LiveMessage } from '$lib/live/protocol';
import { AgentEvent, TransferMode } from './agents';
import {
	AgentsManager,
	LoadState,
	REFRESH_INTERVAL_MS,
	type AgentsApi
} from './agents-manager.svelte';
import { AgentSettingsManager, type AgentSettingsApi } from './agent-settings-manager.svelte';

const SERVER = 'http://magpie.lan:3000';
const SECRET = 'a-secret-that-is-long-enough-0123456789';

function agent(overrides: Partial<AgentResponse> = {}): AgentResponse {
	return {
		id: 'agent-1',
		hostname: 'gpu-box',
		version: '1',
		ffmpegVersion: '7',
		mode: TransferMode.DOWNLOAD_FIRST,
		slotsOverride: 2,
		online: true,
		draining: false,
		lastSeenAt: '',
		scratchFreeBytes: 0,
		throughputBps: 0,
		lifetimeSavedBytes: 0,
		devices: [],
		...overrides
	};
}

function fakeApi(agents: AgentResponse[]) {
	const api = {
		list: vi.fn(async () => agents),
		update: vi.fn(async (id: string, request: UpdateAgentRequest) => ({
			...agent({ id }),
			...request
		})),
		retest: vi.fn(async () => ({}))
	} satisfies AgentsApi;
	return api;
}

const notify = () => ({ success: vi.fn(), error: vi.fn() });

class FakeLive {
	handlers = new Map<string, LiveHandler>();
	reset: ResetListener | undefined;

	on(type: string, handler: LiveHandler) {
		this.handlers.set(type, handler);
		return () => this.handlers.delete(type);
	}

	onReset(listener: ResetListener) {
		this.reset = listener;
		return () => (this.reset = undefined);
	}

	emit(type: string) {
		this.handlers.get(type)?.({ type } as LiveMessage);
	}
}

describe('AgentsManager', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	it('loads agents', async () => {
		const manager = new AgentsManager(fakeApi([agent()]), notify());
		await manager.load();
		expect(manager.loadState).toBe(LoadState.READY);
		expect(manager.agents).toHaveLength(1);
	});

	it('shows an error state when the first load fails', async () => {
		const api = fakeApi([]);
		api.list.mockRejectedValueOnce(new Error('down'));
		const manager = new AgentsManager(api, notify());
		await manager.load();
		expect(manager.loadState).toBe(LoadState.ERROR);
		expect(manager.error).not.toBe('');
	});

	it('sends every override when one changes', async () => {
		const api = fakeApi([agent()]);
		const manager = new AgentsManager(api, notify());
		await manager.load();

		await manager.setDraining(manager.agents[0], true);

		expect(api.update).toHaveBeenCalledWith('agent-1', {
			slotsOverride: 2,
			mode: TransferMode.DOWNLOAD_FIRST,
			draining: true
		});
		expect(manager.agents[0].draining).toBe(true);
		expect(manager.busy.size).toBe(0);
	});

	it('reports a failed action without losing the list', async () => {
		const api = fakeApi([agent()]);
		api.retest.mockRejectedValueOnce(new Error('offline'));
		const notifier = notify();
		const manager = new AgentsManager(api, notifier);
		await manager.load();

		await manager.retest(manager.agents[0]);

		expect(notifier.error).toHaveBeenCalled();
		expect(manager.agents).toHaveLength(1);
	});

	it('reloads on agent events and on the refresh beat until stopped', async () => {
		const api = fakeApi([agent()]);
		const manager = new AgentsManager(api, notify());
		const live = new FakeLive();
		const stop = manager.watch(live as unknown as LiveConnection);

		live.emit(AgentEvent.ONLINE);
		live.emit(AgentEvent.OFFLINE);
		live.reset?.();
		await vi.advanceTimersByTimeAsync(REFRESH_INTERVAL_MS);
		expect(api.list).toHaveBeenCalledTimes(4);

		stop();
		live.emit(AgentEvent.ONLINE);
		await vi.advanceTimersByTimeAsync(REFRESH_INTERVAL_MS);
		expect(api.list).toHaveBeenCalledTimes(4);
	});
});

describe('AgentSettingsManager', () => {
	function settingsApi() {
		return {
			secret: vi.fn(async () => ({ configured: true, masked: '••••6789', secret: SECRET })),
			settings: vi.fn(async () => ({ transferCap: 2, defaultSlotsPerGpu: 1 })),
			save: vi.fn(async (s) => s)
		} satisfies AgentSettingsApi;
	}

	it('saves only valid, changed limits', async () => {
		const api = settingsApi();
		const manager = new AgentSettingsManager(SERVER, api, notify(), () => ({ writeText: vi.fn() }));
		await manager.load();
		expect(manager.canSave).toBe(false);

		manager.transferCap = 17;
		expect(manager.canSave).toBe(false);

		manager.transferCap = 4;
		await manager.save();
		expect(api.save).toHaveBeenCalledWith({ transferCap: 4, defaultSlotsPerGpu: 1 });
		expect(manager.canSave).toBe(false);
	});

	it('copies the real secret, not the mask', async () => {
		const writeText = vi.fn(async () => {});
		const manager = new AgentSettingsManager(SERVER, settingsApi(), notify(), () => ({
			writeText
		}));
		await manager.load();

		await manager.copySecret();

		expect(writeText).toHaveBeenCalledWith(SECRET);
		expect(manager.dockerRun).not.toContain(SECRET);
	});
});
