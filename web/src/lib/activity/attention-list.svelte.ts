import type { AttentionItemResponse } from '$lib/sdk';
import { DataBlock } from '$lib/data-block.svelte';
import { LiveEventType, type LiveEvents } from './activity-source';
import { SDK_FEEDS, type ActivityFeeds } from './feeds';

/** The open attention items: Home's "Needs you" and the Home badge. Admins only. */
export class AttentionList {
	readonly block: DataBlock<AttentionItemResponse[]>;

	constructor(feeds: ActivityFeeds = SDK_FEEDS) {
		this.block = new DataBlock({
			fetch: () => feeds.attention(),
			initial: [],
			isEmpty: (items) => items.length === 0,
			failure: 'Could not load what needs you.'
		});
	}

	get items(): AttentionItemResponse[] {
		return this.block.data;
	}

	get count(): number {
		return this.block.data.length;
	}

	load(): Promise<void> {
		return this.block.load();
	}

	/** Follows AttentionChanged until the returned function is called. */
	watch(live: LiveEvents): () => void {
		const stops = [
			live.on(LiveEventType.ATTENTION_CHANGED, () => void this.block.refresh()),
			live.onReset(() => void this.block.refresh())
		];
		return () => stops.forEach((stop) => stop());
	}
}
