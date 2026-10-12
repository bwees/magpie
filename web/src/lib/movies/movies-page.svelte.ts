import type {
	FileTranscoded,
	MediaFileResponse,
	PipelineRowResponse,
	ProjectionUpdated,
	TitleResponse
} from '$lib/sdk';
import { DataBlock } from '$lib/data-block.svelte';
import { LiveEventType, Projection, type LiveEvents } from '$lib/activity/activity-source';
import { SDK_FEEDS, type ActivityFeeds } from '$lib/activity/feeds';
import { Stage } from '$lib/activity/pipeline';
import {
	CATALOG_EVENTS,
	SDK_TITLE_LISTER,
	TitleSort,
	type TitleLister
} from '$lib/catalog/library-titles';
import { MediaKind } from '$lib/catalog/title-page.svelte';
import { LibraryEvent } from '$lib/library/library';
import { SDK_LIBRARY_FEEDS, byTitle, type LibraryFeeds } from '$lib/library/library-feeds';

export enum MovieFilter {
	ALL = 'ALL',
	MISSING = 'MISSING',
	IN_PROGRESS = 'IN_PROGRESS',
	AVAILABLE = 'AVAILABLE',
	NOT_TRANSCODED = 'NOT_TRANSCODED'
}

export const MOVIE_FILTERS: MovieFilter[] = Object.values(MovieFilter);

export const FILTER_LABELS: Record<MovieFilter, string> = {
	[MovieFilter.ALL]: 'All',
	[MovieFilter.MISSING]: 'Missing',
	[MovieFilter.IN_PROGRESS]: 'In progress',
	[MovieFilter.AVAILABLE]: 'Available',
	[MovieFilter.NOT_TRANSCODED]: 'Not transcoded'
};

export const MOVIE_SORTS: TitleSort[] = [TitleSort.ADDED, TitleSort.NAME, TitleSort.YEAR];

export const SORT_LABELS: Record<TitleSort, string> = {
	[TitleSort.ADDED]: 'Recently added',
	[TitleSort.NAME]: 'Title',
	[TitleSort.YEAR]: 'Year'
};

export enum MoviesLayout {
	GRID = 'GRID',
	TABLE = 'TABLE'
}

/** Stages a movie passes through between being wanted and being watchable. */
const MOVING_STAGES: string[] = [Stage.SEARCHING, Stage.DOWNLOADING, Stage.IMPORTING];

/** One movie with what Activity and Library know about it. */
export interface MovieRow {
	title: TitleResponse;
	stage?: PipelineRowResponse;
	/** Newest first. */
	files: MediaFileResponse[];
}

export function matchesFilter(row: MovieRow, filter: MovieFilter): boolean {
	switch (filter) {
		case MovieFilter.MISSING:
			return row.files.length === 0;
		case MovieFilter.IN_PROGRESS:
			return row.stage !== undefined && MOVING_STAGES.includes(row.stage.stage);
		case MovieFilter.AVAILABLE:
			return row.stage?.stage === Stage.AVAILABLE;
		case MovieFilter.NOT_TRANSCODED:
			return row.files.some((file) => !file.transcoded);
		default:
			return true;
	}
}

type ByTitle<T> = Record<string, T>;

/**
 * The Movies page: Catalog's movies with Activity's stage and Library's files, the filter chips with
 * their counts, the sort and the grid/table toggle. Each source is its own block, so one failing still
 * shows the others.
 */
export class MoviesPage {
	filter = $state(MovieFilter.ALL);
	sort = $state(TitleSort.ADDED);
	layout = $state(MoviesLayout.GRID);

	readonly titles: DataBlock<TitleResponse[]>;
	readonly stages: DataBlock<ByTitle<PipelineRowResponse>>;
	readonly files: DataBlock<ByTitle<MediaFileResponse[]>>;

	readonly rows: MovieRow[];
	readonly shown: MovieRow[];

	readonly #feeds: ActivityFeeds;
	readonly #library: LibraryFeeds;

	constructor(
		lister: TitleLister = SDK_TITLE_LISTER,
		feeds: ActivityFeeds = SDK_FEEDS,
		library: LibraryFeeds = SDK_LIBRARY_FEEDS
	) {
		this.#feeds = feeds;
		this.#library = library;
		this.titles = new DataBlock({
			fetch: () => lister(MediaKind.MOVIE, this.sort),
			initial: [],
			isEmpty: (titles) => titles.length === 0,
			failure: 'Could not load your movies.'
		});
		this.stages = new DataBlock({
			fetch: () => this.#stagesOf(this.#ids()),
			initial: {},
			isEmpty: () => false,
			failure: 'Could not load the pipeline stages.'
		});
		this.files = new DataBlock({
			fetch: () => this.#filesOf(this.#ids()),
			initial: {},
			isEmpty: () => false,
			failure: 'Could not load the files.'
		});
		this.rows = $derived(
			this.titles.data.map((title) => ({
				title,
				stage: this.stages.data[title.id],
				files: this.files.data[title.id] ?? []
			}))
		);
		this.shown = $derived(this.rows.filter((row) => matchesFilter(row, this.filter)));
	}

	#ids(): string[] {
		return this.titles.data.map((title) => title.id);
	}

	async #stagesOf(ids: string[]): Promise<ByTitle<PipelineRowResponse>> {
		return Object.fromEntries((await this.#feeds.stages(ids)).map((row) => [row.titleId, row]));
	}

	async #filesOf(ids: string[]): Promise<ByTitle<MediaFileResponse[]>> {
		return byTitle(await this.#library.files(ids), (file) => file.title);
	}

	/** How many movies a chip holds; undefined until the source it filters on has answered. */
	count(filter: MovieFilter): number | undefined {
		if (!this.#sourceOf(filter).ready) return undefined;
		return this.rows.filter((row) => matchesFilter(row, filter)).length;
	}

	#sourceOf(filter: MovieFilter): { ready: boolean } {
		switch (filter) {
			case MovieFilter.MISSING:
			case MovieFilter.NOT_TRANSCODED:
				return this.files;
			case MovieFilter.IN_PROGRESS:
			case MovieFilter.AVAILABLE:
				return this.stages;
			default:
				return this.titles;
		}
	}

	async load(): Promise<void> {
		await this.titles.load();
		await Promise.all([this.stages.load(), this.files.load()]);
	}

	/** Reloads the titles, then what Activity and Library say about them. */
	async refresh(): Promise<void> {
		await this.titles.refresh();
		await Promise.all([this.stages.refresh(), this.files.refresh()]);
	}

	select(filter: MovieFilter): void {
		this.filter = filter;
	}

	/** Re-sorts on the server; the stages and files stay. */
	sortBy(sort: TitleSort): Promise<void> {
		this.sort = sort;
		return this.titles.refresh();
	}

	async #refreshStage(title: string): Promise<void> {
		try {
			const [row] = await this.#feeds.stages([title]);
			if (row) this.stages.data = { ...this.stages.data, [title]: row };
		} catch {
			// The row keeps its last stage until the next update.
		}
	}

	async #refreshFiles(title: string): Promise<void> {
		try {
			const files = await this.#filesOf([title]);
			this.files.data = { ...this.files.data, [title]: files[title] ?? [] };
		} catch {
			// The row keeps its last files until the next update.
		}
	}

	#known(title: string | null | undefined): title is string {
		return !!title && this.titles.data.some((t) => t.id === title);
	}

	/** Follows catalog, pipeline and file events until the returned function is called. */
	watch(live: LiveEvents): () => void {
		const stops = CATALOG_EVENTS.map((type) => live.on(type, () => void this.refresh()));
		stops.push(
			live.on(LiveEventType.PROJECTION_UPDATED, (message) => {
				const update = message.data as ProjectionUpdated;
				if (update.projection === Projection.PIPELINE && this.#known(update.title)) {
					void this.#refreshStage(update.title);
				}
			}),
			...[LibraryEvent.FILE_IMPORTED, LibraryEvent.FILE_DELETED].map((type) =>
				live.on(type, (message) => {
					const title = (message.data as { title?: string }).title;
					if (this.#known(title)) void this.#refreshFiles(title);
				})
			),
			live.on(LibraryEvent.FILE_TRANSCODED, (message) => {
				const file = (message.data as FileTranscoded).mediaFile;
				const title = Object.keys(this.files.data).find((id) =>
					this.files.data[id].some((f) => f.id === file)
				);
				if (title) void this.#refreshFiles(title);
			}),
			live.onReset(() => void this.refresh())
		);
		return () => stops.forEach((stop) => stop());
	}
}
