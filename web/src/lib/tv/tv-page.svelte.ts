import type { TitleCoverageResponse, TitleResponse } from '$lib/sdk';
import { DataBlock } from '$lib/data-block.svelte';
import type { LiveEvents } from '$lib/activity/activity-source';
import {
	CATALOG_EVENTS,
	SDK_TITLE_LISTER,
	TitleSort,
	type TitleLister
} from '$lib/catalog/library-titles';
import { MediaKind } from '$lib/catalog/title-page.svelte';
import { LibraryEvent } from '$lib/library/library';
import { SDK_LIBRARY_FEEDS, type LibraryFeeds } from '$lib/library/library-feeds';

/** Mirrors the Go `catalog.SeriesStatus*` values. */
export enum SeriesStatus {
	CONTINUING = 'CONTINUING',
	ENDED = 'ENDED'
}

export const SERIES_STATUS_LABELS: Record<string, string> = {
	[SeriesStatus.CONTINUING]: 'Continuing',
	[SeriesStatus.ENDED]: 'Ended'
};

export enum TvFilter {
	ALL = 'ALL',
	MISSING_EPISODES = 'MISSING_EPISODES',
	CONTINUING = 'CONTINUING',
	ENDED = 'ENDED'
}

export const TV_FILTERS: TvFilter[] = Object.values(TvFilter);

export const FILTER_LABELS: Record<TvFilter, string> = {
	[TvFilter.ALL]: 'All',
	[TvFilter.MISSING_EPISODES]: 'Missing episodes',
	[TvFilter.CONTINUING]: 'Continuing',
	[TvFilter.ENDED]: 'Ended'
};

/** Library events that change a series' coverage; each names its title. */
const COVERAGE_EVENTS: string[] = [
	LibraryEvent.FILE_IMPORTED,
	LibraryEvent.FILE_DELETED,
	LibraryEvent.ITEM_AVAILABLE
];

/** One series with its coverage, once Library answered. */
export interface SeriesRow {
	title: TitleResponse;
	coverage?: TitleCoverageResponse;
}

/** Wanted episodes without a file. Files of unwanted episodes never make it negative. */
export function missingEpisodes(row: SeriesRow): number {
	return Math.max(row.title.wantedEpisodeCount - (row.coverage?.episodesWithFile ?? 0), 0);
}

export function matchesFilter(row: SeriesRow, filter: TvFilter): boolean {
	switch (filter) {
		case TvFilter.MISSING_EPISODES:
			return row.coverage !== undefined && missingEpisodes(row) > 0;
		case TvFilter.CONTINUING:
			return row.title.seriesStatus === SeriesStatus.CONTINUING;
		case TvFilter.ENDED:
			return row.title.seriesStatus === SeriesStatus.ENDED;
		default:
			return true;
	}
}

/** The TV page: Catalog's series with Library's coverage, and the filter chips. */
export class TvPage {
	filter = $state(TvFilter.ALL);

	readonly series: DataBlock<TitleResponse[]>;
	readonly coverage: DataBlock<Record<string, TitleCoverageResponse>>;

	readonly rows: SeriesRow[];
	readonly shown: SeriesRow[];
	readonly totalSize: number;

	readonly #library: LibraryFeeds;

	constructor(lister: TitleLister = SDK_TITLE_LISTER, library: LibraryFeeds = SDK_LIBRARY_FEEDS) {
		this.#library = library;
		this.series = new DataBlock({
			fetch: () => lister(MediaKind.SERIES, TitleSort.NAME),
			initial: [],
			isEmpty: (series) => series.length === 0,
			failure: 'Could not load your series.'
		});
		this.coverage = new DataBlock({
			fetch: () => this.#coverageOf(this.series.data.map((title) => title.id)),
			initial: {},
			isEmpty: () => false,
			failure: 'Could not load episode coverage.'
		});
		this.rows = $derived(
			this.series.data.map((title) => ({ title, coverage: this.coverage.data[title.id] }))
		);
		this.shown = $derived(this.rows.filter((row) => matchesFilter(row, this.filter)));
		this.totalSize = $derived(
			Object.values(this.coverage.data).reduce((sum, coverage) => sum + coverage.sizeBytes, 0)
		);
	}

	async #coverageOf(ids: string[]): Promise<Record<string, TitleCoverageResponse>> {
		return Object.fromEntries((await this.#library.coverage(ids)).map((c) => [c.title, c]));
	}

	/** How many series a chip holds; Missing episodes is undefined until coverage has answered. */
	count(filter: TvFilter): number | undefined {
		const source = filter === TvFilter.MISSING_EPISODES ? this.coverage : this.series;
		if (!source.ready) return undefined;
		return this.rows.filter((row) => matchesFilter(row, filter)).length;
	}

	select(filter: TvFilter): void {
		this.filter = filter;
	}

	async load(): Promise<void> {
		await this.series.load();
		await this.coverage.load();
	}

	async refresh(): Promise<void> {
		await this.series.refresh();
		await this.coverage.refresh();
	}

	async #refreshCoverage(title: string): Promise<void> {
		try {
			const coverage = await this.#coverageOf([title]);
			this.coverage.data = { ...this.coverage.data, ...coverage };
		} catch {
			// The row keeps its last coverage until the next update.
		}
	}

	/** Follows catalog and library events until the returned function is called. */
	watch(live: LiveEvents): () => void {
		const stops = CATALOG_EVENTS.map((type) => live.on(type, () => void this.refresh()));
		stops.push(
			...COVERAGE_EVENTS.map((type) =>
				live.on(type, (message) => {
					const title = (message.data as { title?: string }).title;
					if (title && this.series.data.some((series) => series.id === title)) {
						void this.#refreshCoverage(title);
					}
				})
			),
			live.onReset(() => void this.refresh())
		);
		return () => stops.forEach((stop) => stop());
	}
}
