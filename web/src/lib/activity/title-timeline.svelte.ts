import type { ProjectionUpdated, TimelineEntryResponse } from '$lib/sdk';
import { LiveEventType, Projection, type ActivitySource, type LiveEvents } from './activity-source';
import { PagedFeed } from './paged-feed.svelte';

/** A title's timeline, newest first, refetched when Activity records something for the title. */
export class TitleTimeline extends PagedFeed<TimelineEntryResponse> {
	readonly titleId: string;

	constructor(titleId: string, source: ActivitySource) {
		super((before) => source.timeline(titleId, before));
		this.titleId = titleId;
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
