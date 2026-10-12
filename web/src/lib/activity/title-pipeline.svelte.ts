import { SvelteMap } from 'svelte/reactivity';
import {
	apiErrorMessage,
	type DownloadProgressed,
	type JobProgressed,
	type PipelineRowResponse,
	type ProjectionUpdated
} from '$lib/sdk';
import {
	LiveEventType,
	LoadStatus,
	Projection,
	type ActivitySource,
	type LiveEvents
} from './activity-source';
import { Stage, summarize, type PipelineSummary } from './pipeline';

const PERCENT = 100;

function percentOf(progress: number): number {
	return Math.round(progress * PERCENT);
}

/** The stage bar of one title: its pipeline rows, refetched when Activity updates them, plus live percents. */
export class TitlePipeline {
	rows = $state<PipelineRowResponse[]>([]);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	summary: PipelineSummary | undefined = $derived(summarize(this.rows));
	/** A series' rows by episode ID, for the episode tables. */
	byEpisode: Record<string, PipelineRowResponse> = $derived(
		Object.fromEntries(this.rows.flatMap((row) => (row.episodeId ? [[row.episodeId, row]] : [])))
	);

	readonly titleId: string;
	readonly #source: ActivitySource;
	readonly #downloadPercents = new SvelteMap<string, number>();
	readonly #jobPercents = new SvelteMap<string, number>();
	#latest: Promise<PipelineRowResponse[]> | undefined;

	constructor(titleId: string, source: ActivitySource) {
		this.titleId = titleId;
		this.#source = source;
	}

	/** How many of the episodes Jellyfin has confirmed. */
	availableCount(episodeIds: string[]): number {
		return episodeIds.filter((id) => this.byEpisode[id]?.stage === Stage.AVAILABLE).length;
	}

	get empty(): boolean {
		return this.status === LoadStatus.READY && this.rows.length === 0;
	}

	/** The live download percent of the summary row, while it downloads. */
	get downloadPercent(): number | undefined {
		const download = this.summary?.row.downloadId;
		return download ? this.#downloadPercents.get(download) : undefined;
	}

	/** The live percent of the running transcode the chip shows. */
	get transcodePercent(): number | undefined {
		const job = this.summary?.transcode?.jobId;
		return job ? this.#jobPercents.get(job) : undefined;
	}

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		await this.refresh();
	}

	retry(): Promise<void> {
		return this.load();
	}

	async refresh(): Promise<void> {
		const request = this.#source.pipeline(this.titleId);
		this.#latest = request;
		try {
			const rows = await request;
			if (this.#latest !== request) return;
			this.rows = rows;
			this.error = '';
			this.status = LoadStatus.READY;
		} catch (error) {
			if (this.#latest !== request) return;
			this.error = apiErrorMessage(error);
			if (this.rows.length === 0) this.status = LoadStatus.ERROR;
		}
	}

	/** Follows the live socket until the returned function is called. */
	connect(live: LiveEvents): () => void {
		const unsubscribers = [
			live.on(LiveEventType.PROJECTION_UPDATED, (message) => {
				const update = message.data as ProjectionUpdated;
				if (update.projection === Projection.PIPELINE && update.title === this.titleId) {
					void this.refresh();
				}
			}),
			live.on(LiveEventType.DOWNLOAD_PROGRESSED, (message) => {
				const progress = message.data as DownloadProgressed;
				if (this.rows.some((row) => row.downloadId === progress.download)) {
					this.#downloadPercents.set(progress.download, percentOf(progress.progress));
				}
			}),
			live.on(LiveEventType.JOB_PROGRESSED, (message) => {
				const progress = message.data as JobProgressed;
				if (this.rows.some((row) => row.transcode?.jobId === progress.job)) {
					this.#jobPercents.set(progress.job, percentOf(progress.progress));
				}
			}),
			live.onReset(() => void this.refresh())
		];
		return () => unsubscribers.forEach((unsubscribe) => unsubscribe());
	}
}
