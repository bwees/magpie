import { apiErrorMessage } from '$lib/sdk';
import { LoadStatus, PAGE_SIZE } from './activity-source';

export interface FeedEntry {
	eventId: string;
	at: string;
}

/** A newest-first list read a page at a time. A refresh adds new entries on top and keeps older pages. */
export class PagedFeed<T extends FeedEntry> {
	entries = $state<T[]>([]);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	hasMore = $state(false);
	loadingMore = $state(false);
	moreError = $state('');

	readonly #fetchPage: (before?: string) => Promise<T[]>;
	#latest: Promise<T[]> | undefined;

	constructor(fetchPage: (before?: string) => Promise<T[]>) {
		this.#fetchPage = fetchPage;
	}

	get empty(): boolean {
		return this.status === LoadStatus.READY && this.entries.length === 0;
	}

	/** Loads the first page, showing the skeleton. */
	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		this.entries = [];
		await this.refresh();
	}

	retry(): Promise<void> {
		return this.load();
	}

	/** Fetches the newest page and merges it in without blanking the list. */
	async refresh(): Promise<void> {
		const request = this.#fetchPage();
		this.#latest = request;
		try {
			const page = await request;
			if (this.#latest !== request) return;
			const fresh = page.map((entry) => entry.eventId);
			const older = this.entries.filter((entry) => !fresh.includes(entry.eventId));
			if (this.entries.length === 0) this.hasMore = page.length === PAGE_SIZE;
			this.entries = [...page, ...older];
			this.error = '';
			this.status = LoadStatus.READY;
		} catch (error) {
			if (this.#latest !== request) return;
			this.error = apiErrorMessage(error);
			if (this.entries.length === 0) this.status = LoadStatus.ERROR;
		}
	}

	async loadMore(): Promise<void> {
		const oldest = this.entries.at(-1);
		if (!oldest || this.loadingMore) return;
		this.loadingMore = true;
		try {
			const page = await this.#fetchPage(oldest.at);
			this.entries = [...this.entries, ...page];
			this.hasMore = page.length === PAGE_SIZE;
			this.moreError = '';
		} catch (error) {
			this.moreError = apiErrorMessage(error);
		} finally {
			this.loadingMore = false;
		}
	}
}
