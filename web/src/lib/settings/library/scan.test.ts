import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { HttpError } from '@oazapfts/runtime';
import type { DiscoverItemResponse, LibraryScanResponse, ScanReviewResponse } from '$lib/sdk';
import { LibraryEvent, MediaKind } from '$lib/library/library';
import {
	LibraryScanManager,
	SCAN_EVENT_SETTLE_MS,
	SCAN_POLL_MS,
	SCAN_SETTLE_MS,
	type LibraryScanApi,
	type ScanLive
} from './library-scan.svelte';
import { formatScore, ScanReviewManager, type ScanReviewApi } from './scan-review.svelte';

const SERVICE_UNAVAILABLE = 503;
const TMDB_DOWN = 'TMDB is unavailable';
const NOW = Date.parse('2026-10-12T12:00:00Z');

const IDLE: LibraryScanResponse = {
	running: false,
	startedAt: null,
	finishedAt: null,
	error: '',
	reported: 0,
	found: 0,
	linked: 0,
	ignored: 0,
	renameMatched: false
};

const notifier = () => ({ success: vi.fn(), error: vi.fn() });

function fakeLive(): ScanLive & { emit: (type: string) => void } {
	const handlers = new Map<string, () => void>();
	return {
		on: (type, handler) => {
			handlers.set(type, handler as () => void);
			return () => handlers.delete(type);
		},
		onReset: () => () => undefined,
		emit: (type) => handlers.get(type)?.()
	};
}

function scanApi(statuses: LibraryScanResponse[]): LibraryScanApi {
	return {
		status: vi.fn(async () => statuses.shift() ?? IDLE),
		start: vi.fn(async () => ({ ...IDLE, running: true, startedAt: new Date(NOW).toISOString() })),
		setOptions: vi.fn(async (request) => ({ ...IDLE, ...request }))
	};
}

describe('LibraryScanManager', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	it('starts a scan and polls until it settles, reloading the review queue each time', async () => {
		const finishedAt = new Date(NOW).toISOString();
		const api = scanApi([{ ...IDLE, running: false, finishedAt, reported: 3, found: 3 }]);
		const changed = vi.fn();
		const manager = new LibraryScanManager(api, notifier(), () => NOW);
		const stop = manager.watch(fakeLive(), changed);

		await manager.start();
		expect(manager.running).toBe(true);
		await vi.advanceTimersByTimeAsync(SCAN_POLL_MS);

		expect(api.status).toHaveBeenCalledTimes(1);
		expect(changed).toHaveBeenCalledTimes(1);
		expect(manager.scan?.reported).toBe(3);
		expect(manager.settling).toBe(true);
		stop();
	});

	it('stops polling once the scan has settled', async () => {
		const longAgo = new Date(NOW - SCAN_SETTLE_MS - 1).toISOString();
		const api = scanApi([{ ...IDLE, finishedAt: longAgo }]);
		const manager = new LibraryScanManager(api, notifier(), () => NOW);
		const stop = manager.watch(fakeLive(), vi.fn());

		await manager.refresh();
		await vi.advanceTimersByTimeAsync(SCAN_POLL_MS * 2);

		expect(api.status).toHaveBeenCalledTimes(1);
		stop();
	});

	it('refreshes once after a burst of folder events', async () => {
		const api = scanApi([]);
		const live = fakeLive();
		const manager = new LibraryScanManager(api, notifier(), () => NOW);
		const stop = manager.watch(live, vi.fn());

		live.emit(LibraryEvent.FOLDER_FOUND);
		live.emit(LibraryEvent.FOLDER_FOUND);
		await vi.advanceTimersByTimeAsync(SCAN_EVENT_SETTLE_MS);

		expect(api.status).toHaveBeenCalledTimes(1);
		stop();
	});

	it('puts the rename option back when saving fails', async () => {
		const api = scanApi([IDLE]);
		api.setOptions = vi.fn(async () => {
			throw new Error('offline');
		});
		const notify = notifier();
		const manager = new LibraryScanManager(api, notify, () => NOW);
		await manager.load();

		await manager.setRenameMatched(true);

		expect(manager.scan?.renameMatched).toBe(false);
		expect(notify.error).toHaveBeenCalled();
	});
});

const ROW: ScanReviewResponse = {
	id: 'r1',
	folderPath: '/media/movies/Dune',
	kind: MediaKind.MOVIE,
	parsedTitle: 'Dune',
	parsedYear: null,
	tmdbHint: null,
	state: 'PENDING',
	titleId: null,
	createdAt: '2026-10-12T00:00:00Z',
	candidates: [
		{
			tmdbId: 438631,
			kind: MediaKind.MOVIE,
			name: 'Dune',
			year: 2021,
			overview: '',
			posterUrl: null,
			score: 1
		}
	]
};

function reviewApi(): ScanReviewApi {
	return {
		list: vi.fn(async () => [structuredClone(ROW)]),
		match: vi.fn(async () => ({ ...ROW, state: 'MATCHED' })),
		ignore: vi.fn(async () => ({ ...ROW, state: 'IGNORED' })),
		search: vi.fn(async (): Promise<DiscoverItemResponse[]> => [
			{ ...ROW.candidates[0], tmdbId: 841, year: 1984, inLibrary: false, titleId: null },
			{
				...ROW.candidates[0],
				tmdbId: 90228,
				kind: MediaKind.SERIES,
				inLibrary: false,
				titleId: null
			}
		])
	};
}

describe('ScanReviewManager', () => {
	it('matches a folder to a candidate and drops it from the queue', async () => {
		const api = reviewApi();
		const notify = notifier();
		const manager = new ScanReviewManager(api, notify);
		await manager.load();

		await manager.match(manager.rows[0], ROW.candidates[0]);

		expect(api.match).toHaveBeenCalledWith('r1', 438631);
		expect(manager.count).toBe(0);
		expect(notify.success).toHaveBeenCalled();
	});

	it('keeps the row and shows why when matching fails', async () => {
		const api = reviewApi();
		api.match = vi.fn(async () => {
			throw new HttpError(SERVICE_UNAVAILABLE, { detail: TMDB_DOWN }, new Headers());
		});
		const manager = new ScanReviewManager(api, notifier());
		await manager.load();

		await manager.match(manager.rows[0], ROW.candidates[0]);

		expect(manager.count).toBe(1);
		expect(manager.rowErrors.get('r1')).toBe(TMDB_DOWN);
	});

	it('ignores a folder', async () => {
		const api = reviewApi();
		const manager = new ScanReviewManager(api, notifier());
		await manager.load();

		await manager.ignore(manager.rows[0]);

		expect(api.ignore).toHaveBeenCalledWith('r1');
		expect(manager.count).toBe(0);
	});

	it('searches TMDB for other titles of the folder’s kind', async () => {
		const api = reviewApi();
		const manager = new ScanReviewManager(api, notifier());
		await manager.load();
		const row = manager.rows[0];

		manager.setQuery(row, 'Dune 1984');
		await manager.search(row);

		expect(api.search).toHaveBeenCalledWith('Dune 1984');
		expect(manager.candidates(row).map((c) => c.tmdbId)).toEqual([841]);
	});

	it('formats a score as a percentage', () => {
		expect(formatScore(0.934)).toBe('93%');
	});
});
