import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { IndexerListResponse, IndexerResponse } from '$lib/sdk';
import { SeedingOverride } from './indexer-settings';
import { IndexersManager, LoadState } from './indexers-manager.svelte';

const sdk = vi.hoisted(() => ({
	listIndexers: vi.fn(),
	getIndexerOptions: vi.fn(),
	syncIndexers: vi.fn(),
	updateIndexer: vi.fn(),
	setIndexerOptions: vi.fn()
}));

vi.mock('$lib/sdk', () => ({
	...sdk,
	apiErrorMessage: (error: { data?: { detail?: string } }, fallback: string) =>
		error?.data?.detail ?? fallback
}));

const INDEXER: IndexerResponse = {
	id: 2,
	name: 'BeyondHD',
	privacy: 'PRIVATE',
	status: 'OK',
	lastError: '',
	averageResponseMs: 420,
	caps: { movieSearch: true, movieParams: [], tvSearch: true, tvParams: [], categories: [] },
	priority: 25,
	minSeeders: 1,
	seedingOverride: SeedingOverride.USE_GLOBAL
};

const LIST: IndexerListResponse = { indexers: [INDEXER], prowlarrUrl: 'http://prowlarr:9696' };
const OPTIONS = { searchTimeoutSeconds: 60, resultCacheMinutes: 30, syncIntervalMinutes: 60 };

describe('IndexersManager', () => {
	beforeEach(() => {
		vi.resetAllMocks();
		sdk.listIndexers.mockResolvedValue(LIST);
		sdk.getIndexerOptions.mockResolvedValue(OPTIONS);
	});

	it('loads the list and options', async () => {
		const manager = new IndexersManager();
		await manager.load();

		expect(manager.loadState).toBe(LoadState.READY);
		expect(manager.rows.map((row) => row.indexer.name)).toEqual(['BeyondHD']);
		expect(manager.prowlarrUrl).toBe('http://prowlarr:9696');
		expect(manager.options).toEqual(OPTIONS);
		expect(manager.isEmpty).toBe(false);
	});

	it('is empty only when Prowlarr answered with no indexers', async () => {
		sdk.listIndexers.mockResolvedValue({ indexers: [], prowlarrUrl: '' });
		const manager = new IndexersManager();
		await manager.load();
		expect(manager.isEmpty).toBe(true);

		sdk.listIndexers.mockResolvedValue({ indexers: [], prowlarrUrl: '', syncError: 'down' });
		await manager.load();
		expect(manager.isEmpty).toBe(false);
		expect(manager.syncError).toBe('down');
	});

	it('shows a failed sync inline and keeps the list', async () => {
		sdk.syncIndexers.mockRejectedValue({
			data: { detail: 'Prowlarr is unreachable: connection refused' }
		});
		const onSynced = vi.fn();
		const manager = new IndexersManager(onSynced);
		await manager.load();

		await manager.syncNow();

		expect(manager.syncError).toBe('Prowlarr is unreachable: connection refused');
		expect(manager.rows).toHaveLength(1);
		expect(manager.syncing).toBe(false);
		expect(onSynced).not.toHaveBeenCalled();
	});

	it('saves a valid row and refuses an invalid one without calling the server', async () => {
		sdk.updateIndexer.mockResolvedValue({ ...INDEXER, priority: 10 });
		const manager = new IndexersManager();
		await manager.load();
		const [row] = manager.rows;

		row.draft.priority = 99;
		await manager.save(row);
		expect(row.error).toBe('Priority must be 1–50');
		expect(sdk.updateIndexer).not.toHaveBeenCalled();

		row.draft.priority = 10;
		await manager.save(row);
		expect(sdk.updateIndexer).toHaveBeenCalledWith(2, {
			priority: 10,
			minSeeders: 1,
			seedingOverride: SeedingOverride.USE_GLOBAL,
			seedRatio: undefined,
			seedDays: undefined
		});
		expect(row.error).toBeUndefined();
		expect(row.indexer.priority).toBe(10);
	});
});
