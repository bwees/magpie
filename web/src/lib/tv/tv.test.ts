import { describe, expect, it, vi } from 'vitest';
import { CatalogEvent, type TitleLister } from '$lib/catalog/library-titles';
import { MediaKind } from '$lib/catalog/title-page.svelte';
import { LibraryEvent } from '$lib/library/library';
import type { LibraryFeeds } from '$lib/library/library-feeds';
import { LoadStatus } from '$lib/load-status';
import { FakeLive, settle } from '$lib/live/fake-live';
import { coverage, title } from '$lib/testing/fixtures';
import { SeriesStatus, TvFilter, TvPage, missingEpisodes } from './tv-page.svelte';

const EPISODE_SIZE = 1000;

const COMPLETE = title({
	id: 'S1',
	kind: MediaKind.SERIES,
	seriesStatus: SeriesStatus.ENDED,
	wantedEpisodeCount: 10
});
const BEHIND = title({
	id: 'S2',
	kind: MediaKind.SERIES,
	seriesStatus: SeriesStatus.CONTINUING,
	wantedEpisodeCount: 19
});
const SERIES = [COMPLETE, BEHIND];

const COVERAGE = [
	coverage({
		title: 'S1',
		episodesWithFile: 10,
		episodesAvailable: 10,
		sizeBytes: 10 * EPISODE_SIZE
	}),
	coverage({
		title: 'S2',
		episodesWithFile: 14,
		episodesAvailable: 12,
		sizeBytes: 14 * EPISODE_SIZE
	})
];

function setup(
	lister: TitleLister = vi.fn<TitleLister>(async () => SERIES),
	read: LibraryFeeds['coverage'] = vi.fn<LibraryFeeds['coverage']>(async () => COVERAGE)
) {
	return new TvPage(lister, { files: vi.fn(), coverage: read });
}

describe('TV page', () => {
	it('counts missing episodes, filters by chip and totals the size', async () => {
		const page = setup();
		await page.load();
		const [complete, behind] = page.rows;
		expect(missingEpisodes(complete)).toBe(0);
		expect(missingEpisodes(behind)).toBe(5);
		expect(page.count(TvFilter.MISSING_EPISODES)).toBe(1);
		expect(page.count(TvFilter.CONTINUING)).toBe(1);
		expect(page.count(TvFilter.ENDED)).toBe(1);
		expect(page.totalSize).toBe(24 * EPISODE_SIZE);

		page.select(TvFilter.MISSING_EPISODES);
		expect(page.shown.map((row) => row.title.id)).toEqual(['S2']);
	});

	it('never counts files of unwanted episodes as negative missing', () => {
		const row = {
			title: title({ wantedEpisodeCount: 2 }),
			coverage: coverage({ episodesWithFile: 5 })
		};
		expect(missingEpisodes(row)).toBe(0);
	});

	it('keeps the series when coverage fails, with its own error', async () => {
		const page = setup(undefined, async () => Promise.reject(new Error('down')));
		await page.load();
		expect(page.series.status).toBe(LoadStatus.READY);
		expect(page.coverage.status).toBe(LoadStatus.ERROR);
		expect(page.rows).toHaveLength(2);
	});

	it('shows the empty state and the series error', async () => {
		const empty = setup(async () => []);
		await empty.load();
		expect(empty.series.empty).toBe(true);

		const failing = setup(async () => Promise.reject(new Error('down')));
		await failing.load();
		expect(failing.series.status).toBe(LoadStatus.ERROR);
	});

	it('refreshes one series’ coverage when Jellyfin confirms an episode, and the list on catalog events', async () => {
		const lister = vi.fn<TitleLister>(async () => SERIES);
		const read = vi.fn<LibraryFeeds['coverage']>(async () => COVERAGE);
		const page = setup(lister, read);
		const live = new FakeLive();
		await page.load();
		page.watch(live);

		read.mockResolvedValueOnce([
			coverage({ title: 'S2', episodesWithFile: 15, sizeBytes: 15 * EPISODE_SIZE })
		]);
		live.emit(LibraryEvent.ITEM_AVAILABLE, { title: 'S2', episodes: ['E15'] });
		await settle();
		expect(read).toHaveBeenLastCalledWith(['S2']);
		expect(missingEpisodes(page.rows[1])).toBe(4);

		live.emit(CatalogEvent.EPISODES_ANNOUNCED, { title: 'S2' });
		await settle();
		expect(lister).toHaveBeenCalledTimes(2);
	});
});
