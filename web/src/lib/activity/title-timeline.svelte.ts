import type { ProjectionUpdated, TimelineEntryResponse } from '$lib/sdk';
import { LiveEventType, Projection, type ActivitySource, type LiveEvents } from './activity-source';
import { PagedFeed } from './paged-feed.svelte';

export const COLLAPSED_ENTRIES = 4;

/** A title's timeline, newest first, refetched when Activity records something for the title. */
export class TitleTimeline extends PagedFeed<TimelineEntryResponse> {
	readonly titleId: string;
	expanded = $state(false);

	constructor(titleId: string, source: ActivitySource) {
		super((before) => source.timeline(titleId, before));
		this.titleId = titleId;
	}

	get shown(): TimelineEntryResponse[] {
		return this.expanded ? this.entries : this.entries.slice(0, COLLAPSED_ENTRIES);
	}

	get canExpand(): boolean {
		return !this.expanded && this.entries.length > COLLAPSED_ENTRIES;
	}

	/** Older pages load only once the timeline is expanded. */
	get canLoadMore(): boolean {
		return this.hasMore && (this.expanded || this.entries.length <= COLLAPSED_ENTRIES);
	}

	expand(): void {
		this.expanded = true;
	}

	connect(live: LiveEvents): () => void {
		const unsubscribers = [
			live.on(LiveEventType.PROJECTION_UPDATED, (message) => {
				const update = message.data as ProjectionUpdated;
				if (update.projection === Projection.TIMELINE && update.title === this.titleId) {
					void this.refresh();
				}
			}),
			live.onReset(() => void this.refresh())
		];
		return () => unsubscribers.forEach((unsubscribe) => unsubscribe());
	}
}
