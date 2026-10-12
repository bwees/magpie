import { SvelteMap } from 'svelte/reactivity';
import type { DownloadProgressed, JobProgressed, PipelineRowResponse } from '$lib/sdk';
import { LiveEventType, type LiveEvents } from './activity-source';
import { TranscodeState } from './pipeline';

const PERCENT = 100;

export function percentOf(progress: number): number {
	return Math.round(progress * PERCENT);
}

/**
 * Live percents for pipeline rows, from the ephemeral DownloadProgressed and JobProgressed events.
 * They are never stored, so a refetch keeps them.
 */
export class LiveProgress {
	readonly #downloads = new SvelteMap<string, number>();
	readonly #jobs = new SvelteMap<string, number>();
	readonly #rows: () => PipelineRowResponse[];

	/** rows are the rows to follow; progress for anything else is ignored. */
	constructor(rows: () => PipelineRowResponse[]) {
		this.#rows = rows;
	}

	/** The row's live percent: its download's, else its running transcode's. */
	of(row: PipelineRowResponse): number | undefined {
		if (row.downloadId) return this.#downloads.get(row.downloadId);
		const job = row.transcode?.state === TranscodeState.RUNNING ? row.transcode.jobId : null;
		return job ? this.#jobs.get(job) : undefined;
	}

	connect(live: LiveEvents): () => void {
		const stops = [
			live.on(LiveEventType.DOWNLOAD_PROGRESSED, (message) => {
				const progress = message.data as DownloadProgressed;
				if (this.#rows().some((row) => row.downloadId === progress.download)) {
					this.#downloads.set(progress.download, percentOf(progress.progress));
				}
			}),
			live.on(LiveEventType.JOB_PROGRESSED, (message) => {
				const progress = message.data as JobProgressed;
				if (this.#rows().some((row) => row.transcode?.jobId === progress.job)) {
					this.#jobs.set(progress.job, percentOf(progress.progress));
				}
			})
		];
		return () => stops.forEach((stop) => stop());
	}
}
