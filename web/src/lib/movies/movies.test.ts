import { describe, expect, it, vi } from 'vitest';
import type { TitleResponse } from '$lib/sdk';
import { LiveEventType, Projection } from '$lib/activity/activity-source';
import type { ActivityFeeds } from '$lib/activity/feeds';
import { Stage } from '$lib/activity/pipeline';
import { CatalogEvent, TitleSort, type TitleLister } from '$lib/catalog/library-titles';
import { LibraryEvent } from '$lib/library/library';
import type { LibraryFeeds } from '$lib/library/library-feeds';
import { LoadStatus } from '$lib/load-status';
import { FakeLive, settle } from '$lib/live/fake-live';
import { mediaFile, pipelineRow, title } from '$lib/testing/fixtures';
import { MovieFilter, MoviesPage } from './movies-page.svelte';

const MISSING = title({ id: 'M1', name: 'Missing' });
const SEARCHING = title({ id: 'M2', name: 'Searching' });
const AVAILABLE = title({ id: 'M3', name: 'Available' });
const TRANSCODED = title({ id: 'M4', name: 'Transcoded' });
const MOVIES = [MISSING, SEARCHING, AVAILABLE, TRANSCODED];

const STAGES = [
	pipelineRow({ titleId: 'M1', stage: Stage.REQUESTED }),
	pipelineRow({ titleId: 'M2', stage: Stage.SEARCHING }),
	pipelineRow({ titleId: 'M3', stage: Stage.AVAILABLE }),
	pipelineRow({ titleId: 'M4', stage: Stage.AVAILABLE })
];

const FILES = [
	mediaFile({ id: 'F3', title: 'M3', transcoded: false }),
	mediaFile({ id: 'F4', title: 'M4', transcoded: true })
];

function setup(
	overrides: {
		lister?: TitleLister;
		stages?: ActivityFeeds['stages'];
		files?: LibraryFeeds['files'];
	} = {}
) {
	const lister = overrides.lister ?? vi.fn<TitleLister>(async () => MOVIES);
	const stages = overrides.stages ?? vi.fn<ActivityFeeds['stages']>(async () => STAGES);
	const files = overrides.files ?? vi.fn<LibraryFeeds['files']>(async () => FILES);
	const page = new MoviesPage(
		lister,
		{ attention: vi.fn(), active: vi.fn(), recent: vi.fn(), stages },
		{ files, coverage: vi.fn() }
	);
	return { page, lister, stages, files };
}

describe('Movies page', () => {
	it('counts every chip and filters the rows', async () => {
		const { page } = setup();
		await page.load();
		expect(page.count(MovieFilter.ALL)).toBe(4);
		expect(page.count(MovieFilter.MISSING)).toBe(2);
		expect(page.count(MovieFilter.IN_PROGRESS)).toBe(1);
		expect(page.count(MovieFilter.AVAILABLE)).toBe(2);
		expect(page.count(MovieFilter.NOT_TRANSCODED)).toBe(1);

		page.select(MovieFilter.NOT_TRANSCODED);
		expect(page.shown.map((row) => row.title.id)).toEqual(['M3']);
	});

	it('asks Catalog for the chosen sort and keeps the stages and files', async () => {
		const { page, lister, stages } = setup();
		await page.load();
		await page.sortBy(TitleSort.YEAR);
		expect(lister).toHaveBeenLastCalledWith('MOVIE', TitleSort.YEAR);
		expect(stages).toHaveBeenCalledTimes(1);
	});

	it('shows the empty state when there are no movies', async () => {
		const { page } = setup({ lister: async () => [] });
		await page.load();
		expect(page.titles.empty).toBe(true);
	});

	it('keeps the movies when the stages or files fail, with their own error', async () => {
		const { page } = setup({
			stages: async () => Promise.reject(new Error('down')),
			files: async () => Promise.reject(new Error('down'))
		});
		await page.load();
		expect(page.titles.status).toBe(LoadStatus.READY);
		expect(page.stages.status).toBe(LoadStatus.ERROR);
		expect(page.files.status).toBe(LoadStatus.ERROR);
		expect(page.rows).toHaveLength(4);
		expect(page.count(MovieFilter.ALL)).toBe(4);
		expect(page.count(MovieFilter.MISSING)).toBeUndefined();
		expect(page.count(MovieFilter.AVAILABLE)).toBeUndefined();
	});

	it('shows the movies error when Catalog fails', async () => {
		const { page } = setup({ lister: async () => Promise.reject(new Error('down')) });
		await page.load();
		expect(page.titles.status).toBe(LoadStatus.ERROR);
	});

	it('refreshes one movie’s stage and files from live events', async () => {
		const stages = vi.fn<ActivityFeeds['stages']>(async () => STAGES);
		const files = vi.fn<LibraryFeeds['files']>(async () => FILES);
		const { page } = setup({ stages, files });
		const live = new FakeLive();
		await page.load();
		page.watch(live);

		stages.mockResolvedValueOnce([pipelineRow({ titleId: 'M2', stage: Stage.DOWNLOADING })]);
		live.emit(LiveEventType.PROJECTION_UPDATED, { projection: Projection.PIPELINE, title: 'M2' });
		await settle();
		expect(stages).toHaveBeenLastCalledWith(['M2']);
		expect(page.rows.find((row) => row.title.id === 'M2')?.stage?.stage).toBe(Stage.DOWNLOADING);

		files.mockResolvedValueOnce([mediaFile({ id: 'F1', title: 'M1' })]);
		live.emit(LibraryEvent.FILE_IMPORTED, { title: 'M1', mediaFile: 'F1' });
		await settle();
		expect(page.count(MovieFilter.MISSING)).toBe(1);

		files.mockResolvedValueOnce([mediaFile({ id: 'F3', title: 'M3', transcoded: true })]);
		live.emit(LibraryEvent.FILE_TRANSCODED, { mediaFile: 'F3' });
		await settle();
		expect(files).toHaveBeenLastCalledWith(['M3']);
		expect(page.count(MovieFilter.NOT_TRANSCODED)).toBe(1);
	});

	it('reloads the list when Catalog adds a movie', async () => {
		const lister = vi.fn<TitleLister>(async () => MOVIES);
		const { page } = setup({ lister });
		const live = new FakeLive();
		await page.load();
		page.watch(live);

		const added: TitleResponse = title({ id: 'M5' });
		lister.mockResolvedValueOnce([...MOVIES, added]);
		live.emit(CatalogEvent.TITLE_ADDED, { title: 'M5' });
		await settle();
		expect(page.rows).toHaveLength(5);
	});
});
