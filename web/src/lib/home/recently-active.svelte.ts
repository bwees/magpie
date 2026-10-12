import type { PipelineRowResponse, ProjectionUpdated } from '$lib/sdk';
import { DataBlock } from '$lib/data-block.svelte';
import { LiveEventType, Projection, type LiveEvents } from '$lib/activity/activity-source';
import { SDK_FEEDS, type ActivityFeeds } from '$lib/activity/feeds';
import { LiveProgress } from '$lib/activity/live-progress.svelte';
import type { TitleDirectory } from '$lib/catalog/title-directory.svelte';

/** Home's "Recently active" grid: the order and stages from Activity, names and posters from Catalog. */
export class RecentlyActive {
	readonly block: DataBlock<PipelineRowResponse[]>;
	readonly progress: LiveProgress;
	readonly directory: TitleDirectory;

	constructor(directory: TitleDirectory, feeds: ActivityFeeds = SDK_FEEDS) {
		this.directory = directory;
		this.block = new DataBlock({
			fetch: async () => {
				const rows = await feeds.recent();
				await directory.ensure(rows.map((row) => row.titleId));
				return rows;
			},
			initial: [],
			isEmpty: (rows) => rows.length === 0,
			failure: 'Could not load recent activity.'
		});
		this.progress = new LiveProgress(() => this.block.data);
	}

	get rows(): PipelineRowResponse[] {
		return this.block.data;
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
