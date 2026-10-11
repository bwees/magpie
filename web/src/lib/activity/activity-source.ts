import {
	getHistory,
	getPipeline,
	getTimeline,
	type HistoryEntryResponse,
	type PipelineRowResponse,
	type TimelineEntryResponse
} from '$lib/sdk';
import type { LiveHandler, ResetListener } from '$lib/live/live-connection';

/** Matches the server's default page, so a short page means there is nothing older. */
export const PAGE_SIZE = 50;

/** Where Activity's views read from. The SDK in the app, a fake in tests. */
export interface ActivitySource {
	pipeline(titleId: string): Promise<PipelineRowResponse[]>;
	timeline(titleId: string, before?: string): Promise<TimelineEntryResponse[]>;
	history(severity: string | undefined, before?: string): Promise<HistoryEntryResponse[]>;
}

export const SDK_SOURCE: ActivitySource = {
	pipeline: (titleId) => getPipeline({ title: titleId }),
	timeline: (titleId, before) => getTimeline(titleId, { before, limit: PAGE_SIZE }),
	history: (severity, before) => getHistory({ severity, before, limit: PAGE_SIZE })
};

/** The part of the live connection Activity's views use. */
export interface LiveEvents {
	on(type: string, handler: LiveHandler): () => void;
	onReset(listener: ResetListener): () => void;
}

export enum LiveEventType {
	PROJECTION_UPDATED = 'activity.ProjectionUpdated',
	DOWNLOAD_PROGRESSED = 'acquisition.DownloadProgressed',
	JOB_PROGRESSED = 'transcode.JobProgressed'
}

export enum Projection {
	TIMELINE = 'TIMELINE',
	HISTORY = 'HISTORY',
	PIPELINE = 'PIPELINE'
}

export enum LoadStatus {
	LOADING = 'LOADING',
	READY = 'READY',
	ERROR = 'ERROR'
}
