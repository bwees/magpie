import { describe, expect, it, vi } from 'vitest';
import { HttpError } from '@oazapfts/runtime';
import type { LiveHandler, ResetListener } from '$lib/live/live-connection';
import type { LiveMessage } from '$lib/live/protocol';
import { LoadStatus } from '$lib/load-status';
import type { BlocklistEntryResponse, DownloadProgressed, DownloadResponse } from '$lib/sdk';
import { BlocklistManager, type BlocklistApi } from './blocklist-manager.svelte';
import {
	DownloadsManager,
	type DownloadsApi,
	type DownloadsLive
} from './downloads-manager.svelte';
import {
	DownloadEvent,
	DownloadState,
	etaOrGoal,
	formatEta,
	formatPercent,
	formatSpeed,
	nanosecondsToSeconds,
	STATE_LABELS
} from './downloads';

const SERVER_ERROR = 500;
const NANOS_PER_SECOND = 1e9;

function download(id: string, change: Partial<DownloadResponse> = {}): DownloadResponse {
	return {
		id,
		title: 't1',
		titleName: 'Movie Title',
		episodes: [],
		releaseName: `Movie.Title.2024.1080p.WEB-DL-${id}`,
		indexer: 'Nyaa',
		infoHash: 'h',
		quality: { id: 3, name: 'WEBDL-1080p', source: 'WEBDL', resolution: 1080, revision: 1 },
		cfScore: 0,
		state: DownloadState.DOWNLOADING,
		progress: 0.1,
		speedBps: 0,
		etaSeconds: 0,
		ratio: 0,
		sizeBytes: 1024 ** 3,
		seedingMode: 'SEED_UNTIL_GOALS',
		trigger: 'INTERACTIVE',
		override: false,
		grabbedAt: '2026-10-11T12:00:00Z',
		...change
	};
}

function entry(id: string, change: Partial<BlocklistEntryResponse> = {}): BlocklistEntryResponse {
	return {
		id,
		title: 't1',
		titleName: 'Movie Title',
		infoHash: 'h',
		releaseName: `Release-${id}`,
		indexer: 'Nyaa',
		reason: 'BLOCKED_FILE',
		at: '2026-10-11T12:00:00Z',
		...change
	};
}

class FakeLive implements DownloadsLive {
	readonly handlers = new Map<string, Set<LiveHandler>>();
	readonly resets = new Set<ResetListener>();

	on(type: string, handler: LiveHandler): () => void {
		const handlers = this.handlers.get(type) ?? new Set<LiveHandler>();
		handlers.add(handler);
		this.handlers.set(type, handlers);
		return () => handlers.delete(handler);
	}

	onReset(listener: ResetListener): () => void {
		this.resets.add(listener);
		return () => this.resets.delete(listener);
	}

	emit(type: string, data: unknown): void {
		const message: LiveMessage = { seq: 1, epoch: 'E', type, topic: '', data };
		this.handlers.get(type)?.forEach((handler) => handler(message));
	}
}

const notifier = () => ({ success: vi.fn(), error: vi.fn() });

describe('DownloadsManager', () => {
	it('patches progress in place and reloads for downloads it has not seen', async () => {
		const api: DownloadsApi = { list: vi.fn(async () => [download('a')]), remove: vi.fn() };
		const manager = new DownloadsManager(api, notifier());
		const live = new FakeLive();
		await manager.load();
		manager.watch(live);

		const progress: DownloadProgressed = {
			download: 'a',
			state: DownloadState.STALLED,
			progress: 0.5,
			speedBps: 2048,
			eta: 90 * NANOS_PER_SECOND,
			ratio: 0.2
		};
		live.emit(DownloadEvent.PROGRESSED, progress);

		expect(manager.downloads[0]).toMatchObject({
			state: DownloadState.STALLED,
			progress: 0.5,
			speedBps: 2048,
			etaSeconds: 90,
			ratio: 0.2
		});
		expect(api.list).toHaveBeenCalledTimes(1);
		live.emit(DownloadEvent.PROGRESSED, { ...progress, download: 'new' });
		live.emit(DownloadEvent.COMPLETED, {});
		expect(api.list).toHaveBeenCalledTimes(3);
	});

	it('removes a download with or without its data after confirming', async () => {
		const api: DownloadsApi = {
			list: vi.fn(async () => [download('a'), download('b')]),
			remove: vi.fn(async () => undefined)
		};
		const notify = notifier();
		const manager = new DownloadsManager(api, notify);
		await manager.load();

		manager.askRemove(manager.downloads[0]);
		manager.deleteData = false;
		await manager.confirmRemove();

		expect(api.remove).toHaveBeenCalledWith('a', false);
		expect(manager.downloads.map((d) => d.id)).toEqual(['b']);
		expect(manager.confirming).toBeUndefined();
		expect(notify.success).toHaveBeenCalled();
	});

	it('shows a load error, and keeps the list when a reload fails', async () => {
		const list = vi
			.fn()
			.mockRejectedValueOnce(new HttpError(SERVER_ERROR, { detail: 'boom' }, new Headers()));
		const manager = new DownloadsManager({ list, remove: vi.fn() }, notifier());

		await manager.load();
		expect(manager.status).toBe(LoadStatus.ERROR);
		expect(manager.error).toBe('boom');

		list.mockResolvedValueOnce([download('a')]);
		await manager.load();
		list.mockRejectedValueOnce(new Error('offline'));
		await manager.load();
		expect(manager.status).toBe(LoadStatus.READY);
		expect(manager.downloads).toHaveLength(1);
	});
});

describe('BlocklistManager', () => {
	it('lists entries, removes one and reloads on rejections', async () => {
		const api: BlocklistApi = {
			list: vi.fn(async () => [entry('a'), entry('b')]),
			remove: vi.fn(async () => undefined)
		};
		const manager = new BlocklistManager(api, notifier());
		const live = new FakeLive();
		await manager.load();
		manager.watch(live);

		await manager.remove(manager.entries[0]);
		expect(api.remove).toHaveBeenCalledWith('a');
		expect(manager.entries.map((e) => e.id)).toEqual(['b']);

		live.emit(DownloadEvent.REJECTED, {});
		expect(api.list).toHaveBeenCalledTimes(2);
	});

	it('tells expired temporary entries apart', () => {
		const manager = new BlocklistManager({ list: vi.fn(), remove: vi.fn() }, notifier());
		const now = Date.parse('2026-10-12T12:00:00Z');
		expect(manager.isExpired(entry('p'), now)).toBe(false);
		expect(manager.isExpired(entry('t', { expiresAt: '2026-10-12T11:00:00Z' }), now)).toBe(true);
		expect(manager.isExpired(entry('t', { expiresAt: '2026-10-13T11:00:00Z' }), now)).toBe(false);
	});
});

describe('download formatting', () => {
	it('formats speed, ETA and progress', () => {
		expect(formatSpeed(0)).toBe('—');
		expect(formatSpeed(2048)).toBe('2 KB/s');
		expect(formatSpeed(5 * 1024 ** 2)).toBe('5.0 MB/s');
		expect(formatEta(0)).toBe('—');
		expect(formatEta(45)).toBe('45s');
		expect(formatEta(13 * 60 + 5)).toBe('13m');
		expect(formatEta(2 * 3600 + 5 * 60)).toBe('2h 5m');
		expect(formatPercent(0.999)).toBe('99%');
		expect(nanosecondsToSeconds(1.5e9)).toBe(2);
	});

	it('shows the ETA, the seeding mode, or that a download waits for its import', () => {
		expect(etaOrGoal(download('a', { etaSeconds: 45 }))).toBe('45s');
		expect(etaOrGoal(download('b', { state: DownloadState.SEEDING }))).toBe('Seed to goal');
		expect(etaOrGoal(download('c', { state: DownloadState.IMPORT_PENDING_USER }))).toBe(
			'Kept until mapped'
		);
		expect(STATE_LABELS[DownloadState.IMPORT_PENDING_USER]).toBe('Needs mapping');
	});
});
