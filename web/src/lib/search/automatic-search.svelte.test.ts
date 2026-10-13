import { describe, expect, it, vi } from 'vitest';
import { HttpError } from '@oazapfts/runtime';
import { AutomaticSearch, PROGRESS_HINT } from './automatic-search.svelte';
import type { SearchTarget } from './search-target';

const BAD_GATEWAY = 502;
const MOVIE: SearchTarget = { titleId: 't1', label: 'Dune' };
const EPISODE: SearchTarget = { titleId: 't2', episodeIds: ['e3'], label: 'S01E03' };

const notifier = () => ({ success: vi.fn(), error: vi.fn() });

describe('AutomaticSearch', () => {
	it('starts the search and says so', async () => {
		const start = vi.fn(async () => ({}));
		const notify = notifier();
		const search = new AutomaticSearch({ start }, notify);

		await search.search(EPISODE);

		expect(start).toHaveBeenCalledWith(EPISODE);
		expect(notify.success).toHaveBeenCalledWith('Searching for S01E03', {
			description: PROGRESS_HINT
		});
	});

	it('is busy only while its own search starts, and ignores a second click', async () => {
		let answer: () => void = () => {};
		const start = vi.fn(() => new Promise<void>((resolve) => (answer = resolve)));
		const search = new AutomaticSearch({ start }, notifier());

		const first = search.search(MOVIE);
		void search.search(MOVIE);
		expect(search.isStarting(MOVIE)).toBe(true);
		expect(search.isStarting(EPISODE)).toBe(false);
		expect(start).toHaveBeenCalledTimes(1);

		answer();
		await first;
		expect(search.isStarting(MOVIE)).toBe(false);
	});

	it('shows the server error', async () => {
		const failure = new HttpError(
			BAD_GATEWAY,
			{ detail: 'Prowlarr is unreachable' },
			new Headers()
		);
		const notify = notifier();
		const search = new AutomaticSearch(
			{ start: vi.fn(async () => Promise.reject(failure)) },
			notify
		);

		await search.search(MOVIE);

		expect(notify.error).toHaveBeenCalledWith('Prowlarr is unreachable');
		expect(notify.success).not.toHaveBeenCalled();
		expect(search.isStarting(MOVIE)).toBe(false);
	});
});
