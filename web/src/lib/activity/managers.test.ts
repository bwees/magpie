import { describe, expect, it, vi } from 'vitest';
import type { LiveHandler, ResetListener } from '$lib/live/live-connection';
import type { LiveMessage } from '$lib/live/protocol';
import type { HistoryEntryResponse, PipelineRowResponse, TimelineEntryResponse } from '$lib/sdk';
import {
	LiveEventType,
	LoadStatus,
	PAGE_SIZE,
	Projection,
	type ActivitySource,
	type LiveEvents
} from './activity-source';
import { ActivityHistory, HistoryFilter } from './activity-history.svelte';
import { Severity, Stage, TranscodeState } from './pipeline';
import { TitlePipeline } from './title-pipeline.svelte';
import { TitleTimeline } from './title-timeline.svelte';

const TITLE = '01TITLE';
const OTHER_TITLE = '01OTHER';
const DOWNLOAD = '01DOWNLOAD';
const JOB = '01JOB';
const AT = '2026-10-11T12:00:00Z';
const HALF = 0.5;
const HALF_PERCENT = 50;

class FakeLive implements LiveEvents {
	readonly handlers = new Map<string, Set<LiveHandler>>();
	readonly resets = new Set<ResetListener>();

	on(type: string, handler: LiveHandler): () => void {
		const handlers = this.handlers.get(type) ?? new Set<LiveHandler>();
		handlers.add(handler);
		this.handlers.set(type, handlers);
		return () => handlers.delete(handler);
	}

	onReset(listener: ResetListener): () => void {
		this.resets.add(listener);
		return () => this.resets.delete(listener);
	}

	emit(type: string, data: unknown): void {
		const message: LiveMessage = { seq: 1, epoch: 'E', type, topic: '', data };
		this.handlers.get(type)?.forEach((handler) => handler(message));
	}
}

function row(overrides: Partial<PipelineRowResponse> = {}): PipelineRowResponse {
	return {
		titleId: TITLE,
		episodeId: null,
		season: 0,
		number: 0,
		downloadId: null,
		stage: Stage.REQUESTED,
		failed: false,
		detail: '',
		updatedAt: AT,
		...overrides
	};
}

function entry(eventId: string): TimelineEntryResponse {
	return {
		eventId,
		at: AT,
		type: 'catalog.TitleAdded',
		severity: Severity.INFO,
		episodes: [],
		text: eventId,
		detail: ''
	};
}

function source(overrides: Partial<ActivitySource> = {}): ActivitySource {
	return {
		pipeline: vi.fn(async () => []),
		timeline: vi.fn(async () => []),
		history: vi.fn(async () => []),
		...overrides
	};
}

const settle = () => new Promise((resolve) => setTimeout(resolve));

describe('TitlePipeline', () => {
	it('gives each episode row its stage and counts the available ones', async () => {
		const pipeline = vi.fn(async () => [
			row({ episodeId: 'E1', season: 1, number: 1, stage: Stage.AVAILABLE }),
			row({ episodeId: 'E2', season: 1, number: 2, stage: Stage.DOWNLOADING })
		]);
		const manager = new TitlePipeline(TITLE, source({ pipeline }));
		await manager.load();
		expect(manager.byEpisode['E2']?.stage).toBe(Stage.DOWNLOADING);
		expect(manager.availableCount(['E1', 'E2', 'E3'])).toBe(1);
	});

	it('refetches only for its own pipeline updates', async () => {
		const pipeline = vi.fn(async () => [row()]);
		const live = new FakeLive();
		const manager = new TitlePipeline(TITLE, source({ pipeline }));
		await manager.load();
		manager.connect(live);

		live.emit(LiveEventType.PROJECTION_UPDATED, {
			projection: Projection.PIPELINE,
			title: OTHER_TITLE
		});
		live.emit(LiveEventType.PROJECTION_UPDATED, { projection: Projection.TIMELINE, title: TITLE });
		live.emit(LiveEventType.PROJECTION_UPDATED, { projection: Projection.PIPELINE, title: TITLE });
		await settle();

		expect(pipeline).toHaveBeenCalledTimes(2);
		expect(manager.summary?.row.stage).toBe(Stage.REQUESTED);
	});

	it('overlays live download and transcode percents', async () => {
		const live = new FakeLive();
		const running = { state: TranscodeState.RUNNING, jobId: JOB, savedPercent: null, reason: '' };
		const manager = new TitlePipeline(
			TITLE,
			source({
				pipeline: async () => [
					row({ stage: Stage.DOWNLOADING, downloadId: DOWNLOAD, transcode: running })
				]
			})
		);
		await manager.load();
		manager.connect(live);

		live.emit(LiveEventType.DOWNLOAD_PROGRESSED, { download: DOWNLOAD, progress: HALF });
		live.emit(LiveEventType.JOB_PROGRESSED, { job: JOB, progress: HALF });

		expect(manager.downloadPercent).toBe(HALF_PERCENT);
		expect(manager.transcodePercent).toBe(HALF_PERCENT);
	});

	it('shows the error when a reload fails, and keeps the rows when a refetch fails', async () => {
		const pipeline = vi.fn().mockResolvedValueOnce([row()]).mockRejectedValue(new Error('down'));
		const manager = new TitlePipeline(TITLE, source({ pipeline }));
		await manager.load();

		await manager.refresh();
		expect(manager.status).toBe(LoadStatus.READY);

		await manager.load();
		expect(manager.status).toBe(LoadStatus.ERROR);
	});

	it('shows an error until a retry succeeds', async () => {
		const pipeline = vi.fn().mockRejectedValueOnce(new Error('down')).mockResolvedValue([]);
		const manager = new TitlePipeline(TITLE, source({ pipeline }));

		await manager.load();
		expect(manager.status).toBe(LoadStatus.ERROR);

		await manager.retry();
		expect(manager.empty).toBe(true);
	});
});

describe('TitleTimeline', () => {
	it('adds new entries on top and keeps older pages', async () => {
		const firstPage = Array.from({ length: PAGE_SIZE }, (_, i) => entry(`e${PAGE_SIZE - i}`));
		const timeline = vi
			.fn()
			.mockResolvedValueOnce(firstPage)
			.mockResolvedValueOnce([entry('older')])
			.mockResolvedValueOnce([entry('newest'), ...firstPage.slice(0, -1)]);
		const live = new FakeLive();
		const manager = new TitleTimeline(TITLE, source({ timeline }));
		await manager.load();
		expect(manager.hasMore).toBe(true);

		await manager.loadMore();
		expect(manager.hasMore).toBe(false);
		manager.connect(live);
		live.emit(LiveEventType.PROJECTION_UPDATED, { projection: Projection.TIMELINE, title: TITLE });
		await settle();

		expect(manager.entries[0].eventId).toBe('newest');
		expect(manager.entries.at(-1)?.eventId).toBe('older');
		expect(manager.entries).toHaveLength(PAGE_SIZE + 2);
	});
});

describe('ActivityHistory', () => {
	it('filters to warnings and errors', async () => {
		const history = vi.fn(async (): Promise<HistoryEntryResponse[]> => []);
		const manager = new ActivityHistory(source({ history }));

		await manager.setFilter(HistoryFilter.PROBLEMS);

		expect(history).toHaveBeenLastCalledWith(Severity.WARN, undefined);
		expect(manager.feed.empty).toBe(true);
	});
});
