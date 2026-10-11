import {
	discoverTitles,
	listTitles,
	type DiscoverItemResponse,
	type DiscoverResponse,
	type TitleResponse
} from '$lib/sdk';

export enum LoadState {
	IDLE = 'IDLE',
	LOADING = 'LOADING',
	READY = 'READY',
	FAILED = 'FAILED'
}

export const LIBRARY_HIT_LIMIT = 4;
export const SEARCH_DEBOUNCE_MS = 250;

/** Where the search reads from: the library and TMDB, both through Catalog. */
export interface SearchSource {
	library(query: string, signal: AbortSignal): Promise<TitleResponse[]>;
	discover(query: string, signal: AbortSignal): Promise<DiscoverResponse>;
}

export const CATALOG_SOURCE: SearchSource = {
	library: (q, signal) => listTitles({ q }, { signal }),
	discover: (q, signal) => discoverTitles({ q }, { signal })
};

/**
 * The top-bar search. An empty query shows TMDB's trending titles; typing shows library hits and TMDB
 * results. Each load cancels the one before it, so a slow answer never replaces a newer one.
 */
export class TitleSearch {
	query = $state('');
	open = $state(false);
	libraryHits = $state<TitleResponse[]>([]);
	tmdbItems = $state<DiscoverItemResponse[]>([]);
	trending = $state(true);
	tmdbState = $state(LoadState.IDLE);

	readonly #source: SearchSource;
	#inFlight: AbortController | undefined;
	#debounce: ReturnType<typeof setTimeout> | undefined;

	constructor(source: SearchSource = CATALOG_SOURCE) {
		this.#source = source;
	}

	get hasQuery(): boolean {
		return this.query.trim() !== '';
	}

	show(): void {
		this.open = true;
		if (this.tmdbState === LoadState.IDLE) {
			void this.load();
		}
	}

	close(): void {
		clearTimeout(this.#debounce);
		this.#inFlight?.abort();
		this.open = false;
		this.query = '';
		this.libraryHits = [];
		this.tmdbItems = [];
		this.tmdbState = LoadState.IDLE;
	}

	type(query: string): void {
		this.query = query;
		this.open = true;
		clearTimeout(this.#debounce);
		this.#debounce = setTimeout(() => void this.load(), SEARCH_DEBOUNCE_MS);
	}

	retry(): Promise<void> {
		return this.load();
	}

	async load(): Promise<void> {
		this.#inFlight?.abort();
		const request = new AbortController();
		this.#inFlight = request;
		const query = this.query.trim();
		this.tmdbState = LoadState.LOADING;
		await Promise.all([
			this.#loadLibrary(query, request.signal),
			this.#loadTmdb(query, request.signal)
		]);
	}

	async #loadLibrary(query: string, signal: AbortSignal): Promise<void> {
		if (query === '') {
			this.libraryHits = [];
			return;
		}
		try {
			const hits = await this.#source.library(query, signal);
			if (!signal.aborted) {
				this.libraryHits = hits.slice(0, LIBRARY_HIT_LIMIT);
			}
		} catch {
			if (!signal.aborted) {
				this.libraryHits = [];
			}
		}
	}

	async #loadTmdb(query: string, signal: AbortSignal): Promise<void> {
		try {
			const result = await this.#source.discover(query, signal);
			if (signal.aborted) {
				return;
			}
			this.tmdbItems = result.items;
			this.trending = result.trending;
			this.tmdbState = LoadState.READY;
		} catch {
			if (!signal.aborted) {
				this.tmdbItems = [];
				this.tmdbState = LoadState.FAILED;
			}
		}
	}
}
