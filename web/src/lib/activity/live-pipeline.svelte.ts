import type { PipelineRowResponse, ProjectionUpdated } from '$lib/sdk';
import { DataBlock } from '$lib/data-block.svelte';
import { LiveEventType, Projection, type LiveEvents } from './activity-source';
import { SDK_FEEDS, type ActivityFeeds } from './feeds';
import { LiveProgress } from './live-progress.svelte';

/**
 * Rows on the move: Home's "Live now" and the Activity badge. Stage changes refetch the list; download
 * and transcode percents come from the progress events without a refetch.
 */
export class LivePipeline {
	readonly block: DataBlock<PipelineRowResponse[]>;
	readonly progress: LiveProgress;

	constructor(feeds: ActivityFeeds = SDK_FEEDS) {
		this.block = new DataBlock({
			fetch: () => feeds.active(),
			initial: [],
			isEmpty: (rows) => rows.length === 0,
			failure: 'Could not load what is running.'
		});
		this.progress = new LiveProgress(() => this.block.data);
	}

	get rows(): PipelineRowResponse[] {
		return this.block.data;
	}

	get count(): number {
		return this.block.data.length;
	}

	load(): Promise<void> {
		return this.block.load();
	}

	watch(live: LiveEvents): () => void {
		const stops = [
			live.on(LiveEventType.PROJECTION_UPDATED, (message) => {
				if ((message.data as ProjectionUpdated).projection === Projection.PIPELINE) {
					void this.block.refresh();
				}
			}),
			this.progress.connect(live),
			live.onReset(() => void this.block.refresh())
		];
		return () => stops.forEach((stop) => stop());
	}
}
