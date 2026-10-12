import { describe, expect, it, vi } from 'vitest';
import { HttpError } from '@oazapfts/runtime';
import { LoadStatus } from '$lib/load-status';
import { SearchOptionsManager } from './search-options-manager.svelte';

const SERVER_ERROR = 500;

describe('SearchOptionsManager', () => {
	it('loads the switch and saves a change at once', async () => {
		const set = vi.fn(async (options: { searchOnAdd: boolean }) => options);
		const manager = new SearchOptionsManager(
			{ get: vi.fn(async () => ({ searchOnAdd: true })), set },
			{ error: vi.fn() }
		);

		await manager.load();
		expect(manager.status).toBe(LoadStatus.READY);
		expect(manager.saved?.searchOnAdd).toBe(true);

		await manager.setSearchOnAdd(false);
		expect(set).toHaveBeenCalledWith({ searchOnAdd: false });
		expect(manager.saved?.searchOnAdd).toBe(false);
	});

	it('keeps the stored value when saving fails', async () => {
		const notify = { error: vi.fn() };
		const manager = new SearchOptionsManager(
			{
				get: vi.fn(async () => ({ searchOnAdd: true })),
				set: vi.fn(async () => Promise.reject(new HttpError(SERVER_ERROR, {}, new Headers())))
			},
			notify
		);
		await manager.load();

		await manager.setSearchOnAdd(false);

		expect(manager.saved?.searchOnAdd).toBe(true);
		expect(notify.error).toHaveBeenCalledOnce();
		expect(manager.saving).toBe(false);
	});
});
