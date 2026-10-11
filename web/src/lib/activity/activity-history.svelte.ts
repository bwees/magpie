import type { HistoryEntryResponse, ProjectionUpdated } from '$lib/sdk';
import { LiveEventType, Projection, type ActivitySource, type LiveEvents } from './activity-source';
import { PagedFeed } from './paged-feed.svelte';
import { Severity } from './pipeline';

export enum HistoryFilter {
	ALL = 'ALL',
	PROBLEMS = 'PROBLEMS'
}

const FILTER_SEVERITY: Record<HistoryFilter, Severity | undefined> = {
	[HistoryFilter.ALL]: undefined,
	[HistoryFilter.PROBLEMS]: Severity.WARN
};

/** Activity → History: the global feed, optionally only warnings and errors. */
export class ActivityHistory {
	readonly #source: ActivitySource;

	filter = $state(HistoryFilter.ALL);
	feed = $state(this.#feedFor(HistoryFilter.ALL));

	constructor(source: ActivitySource) {
		this.#source = source;
	}

	setFilter(filter: HistoryFilter): Promise<void> {
		this.filter = filter;
		this.feed = this.#feedFor(filter);
		return this.feed.load();
	}

	connect(live: LiveEvents): () => void {
		const unsubscribers = [
			live.on(LiveEventType.PROJECTION_UPDATED, (message) => {
				if ((message.data as ProjectionUpdated).projection === Projection.HISTORY) {
					void this.feed.refresh();
				}
			}),
			live.onReset(() => void this.feed.refresh())
		];
		return () => unsubscribers.forEach((unsubscribe) => unsubscribe());
	}

	#feedFor(filter: HistoryFilter): PagedFeed<HistoryEntryResponse> {
		return new PagedFeed((before) => this.#source.history(FILTER_SEVERITY[filter], before));
	}
}
