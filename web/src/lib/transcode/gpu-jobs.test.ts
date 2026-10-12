import { describe, expect, it, vi } from 'vitest';
import type { JobListResponse, JobProgressed, JobResponse } from '$lib/sdk';
import type { LiveHandler, ResetListener } from '$lib/live/live-connection';
import type { LiveMessage } from '$lib/live/protocol';
import { LoadStatus } from '$lib/load-status';
import { GpuJobsManager, type GpuJobsApi } from './gpu-jobs-manager.svelte';
import { JobEvent, JobState, detailLabel, jobName, kindLabel, stateLabel } from './gpu-jobs';
import { SkipReason } from './transcode-settings';

const NANOS_PER_SECOND = 1e9;
const SERVER_ERROR = 500;

function job(id: string, change: Partial<JobResponse> = {}): JobResponse {
	return {
		id,
		kind: 'TRANSCODE',
		state: JobState.QUEUED,
		mediaFile: `file-${id}`,
		title: 't1',
		titleName: 'Dune',
		episodes: '',
		fileName: 'Dune (2021).mkv',
		origin: 'DOWNLOAD',
		targetCodec: 'HEVC_10',
		language: '',
		agent: null,
		agentName: '',
		attempt: 1,
		maxAttempts: 3,
		progress: 0,
		prefetched: false,
		error: '',
		queuedAt: '2026-10-12T00:00:00Z',
		notBefore: null,
		finishedAt: null,
		...change
	};
}

function list(jobs: JobResponse[], backlog = 0): JobListResponse {
	const counts: Record<string, number> = {};
	for (const j of jobs) counts[j.state] = (counts[j.state] ?? 0) + 1;
	return { jobs, counts, backlog, skipped: {} };
}

function fakeApi(jobs: JobResponse[], backlog = 0) {
	return {
		list: vi.fn(async () => list(jobs, backlog)),
		cancel: vi.fn(async (id: string) => job(id, { state: JobState.CANCELLED })),
		retry: vi.fn(async (id: string) => job(id, { state: JobState.QUEUED }))
	} satisfies GpuJobsApi;
}

const notify = () => ({ success: vi.fn(), error: vi.fn() });

class FakeLive {
	handlers = new Map<string, LiveHandler>();
	reset: ResetListener | undefined;

	on(type: string, handler: LiveHandler) {
		this.handlers.set(type, handler);
		return () => this.handlers.delete(type);
	}

	onReset(listener: ResetListener) {
		this.reset = listener;
		return () => (this.reset = undefined);
	}

	emit(type: string, data: unknown = {}) {
		this.handlers.get(type)?.({ type, data } as LiveMessage);
	}
}

describe('GpuJobsManager', () => {
	it('counts skipped files and those waiting for a capable agent', async () => {
		const api = fakeApi([]);
		api.list.mockResolvedValueOnce({
			...list([]),
			skipped: { [SkipReason.NO_CAPABLE_AGENT]: 3, [SkipReason.LOW_BITRATE]: 2 }
		});
		const manager = new GpuJobsManager(api, notify());

		await manager.load();

		expect(manager.waitingForAgent).toBe(3);
		expect(manager.skippedCount).toBe(5);
		expect(manager.skipReasons).toEqual([SkipReason.LOW_BITRATE, SkipReason.NO_CAPABLE_AGENT]);
	});

	it('splits the list into running, queued and failed jobs', async () => {
		const manager = new GpuJobsManager(
			fakeApi(
				[
					job('a', { state: JobState.ENCODING }),
					job('b'),
					job('c', { state: JobState.FAILED }),
					job('d', { state: JobState.NOT_WORTH_IT })
				],
				1
			),
			notify()
		);
		await manager.load();
		expect(manager.status).toBe(LoadStatus.READY);
		expect(manager.running.map((j) => j.id)).toEqual(['a']);
		expect(manager.queued.map((j) => j.id)).toEqual(['b']);
		expect(manager.failed.map((j) => j.id)).toEqual(['c', 'd']);
		expect(manager.queuedCount).toBe(1);
		expect(manager.backlog).toBe(1);
	});

	it('shows an error when the first load fails', async () => {
		const api = fakeApi([]);
		api.list.mockRejectedValueOnce(new Error(String(SERVER_ERROR)));
		const manager = new GpuJobsManager(api, notify());
		await manager.load();
		expect(manager.status).toBe(LoadStatus.ERROR);
		expect(manager.error).not.toBe('');
	});

	it('patches progress in place and reloads on lifecycle events', async () => {
		const api = fakeApi([job('a', { state: JobState.LEASED })]);
		const manager = new GpuJobsManager(api, notify());
		await manager.load();
		const live = new FakeLive();
		const stop = manager.watch(live);

		const progressed: JobProgressed = {
			job: 'a',
			mediaFile: 'file-a',
			stage: JobState.ENCODING,
			progress: 0.4,
			speed: 2.5,
			eta: 90 * NANOS_PER_SECOND
		};
		live.emit(JobEvent.PROGRESSED, progressed);
		expect(manager.jobs[0].state).toBe(JobState.ENCODING);
		expect(manager.jobs[0].progress).toBe(0.4);
		expect(manager.details.get('a')).toEqual({ speed: 2.5, etaSeconds: 90 });
		expect(api.list).toHaveBeenCalledTimes(1);

		for (const event of [JobEvent.QUEUED, JobEvent.STARTED, JobEvent.FAILED, JobEvent.VERIFIED]) {
			live.emit(event);
		}
		live.reset?.();
		expect(api.list).toHaveBeenCalledTimes(6);
		stop();
		expect(live.handlers.size).toBe(0);
	});

	it('reloads when progress arrives for a job it does not list', async () => {
		const api = fakeApi([]);
		const manager = new GpuJobsManager(api, notify());
		await manager.load();
		const live = new FakeLive();
		manager.watch(live);
		live.emit(JobEvent.PROGRESSED, {
			job: 'unknown',
			stage: JobState.ENCODING,
			progress: 0,
			speed: 0,
			eta: 0
		});
		expect(api.list).toHaveBeenCalledTimes(2);
	});

	it('cancels and retries jobs', async () => {
		const api = fakeApi([
			job('a', { state: JobState.ENCODING }),
			job('b', { state: JobState.FAILED })
		]);
		const toasts = notify();
		const manager = new GpuJobsManager(api, toasts);
		await manager.load();

		await manager.cancel(manager.jobs[0]);
		expect(api.cancel).toHaveBeenCalledWith('a');
		expect(manager.jobs.map((j) => j.id)).toEqual(['b']);

		await manager.retry(manager.jobs[0]);
		expect(api.retry).toHaveBeenCalledWith('b');
		expect(manager.queued.map((j) => j.id)).toEqual(['b']);
		expect(toasts.success).toHaveBeenCalledTimes(2);
	});

	it('reports a failed action and frees the button', async () => {
		const api = fakeApi([job('a', { state: JobState.FAILED })]);
		api.retry.mockRejectedValueOnce(new Error('conflict'));
		const toasts = notify();
		const manager = new GpuJobsManager(api, toasts);
		await manager.load();

		await manager.retry(manager.jobs[0]);
		expect(toasts.error).toHaveBeenCalledOnce();
		expect(manager.busy.size).toBe(0);
	});
});

describe('gpu job labels', () => {
	it('names the kind with its target', () => {
		expect(kindLabel({ kind: 'TRANSCODE', targetCodec: 'HEVC_10', language: '' })).toBe(
			'Transcode · HEVC 10-bit'
		);
		expect(kindLabel({ kind: 'WHISPER', targetCodec: '', language: 'en' })).toBe(
			'Whisper · English'
		);
		expect(kindLabel({ kind: 'WHISPER', targetCodec: '', language: '' })).toBe('Whisper');
	});

	it('adds the episode to the title', () => {
		expect(jobName({ titleName: 'Show', episodes: 'S01E02', fileName: 'x.mkv' })).toBe(
			'Show S01E02'
		);
		expect(jobName({ titleName: '', episodes: '', fileName: 'x.mkv' })).toBe('x.mkv');
	});

	it('shows encode speed as realtime and transfers as bytes per second', () => {
		const encoding = { state: JobState.ENCODING, prefetched: false, progress: 0.5 };
		const uploading = { state: JobState.UPLOADING, prefetched: false, progress: 0.5 };
		expect(detailLabel(encoding, { speed: 2.5, etaSeconds: 90 })).toBe('2.5× realtime · 1m left');
		expect(detailLabel(uploading, { speed: 5 * 1024 ** 2, etaSeconds: 30 })).toBe(
			'5.0 MB/s · 30s left'
		);
		expect(detailLabel(encoding, undefined)).toBe('—');
	});

	it('shows a fetched prefetch as waiting for its slot', () => {
		const prefetched = { state: JobState.FETCHING, prefetched: true, progress: 1 };
		expect(stateLabel(prefetched)).toBe('Prefetched');
		expect(detailLabel(prefetched, undefined)).toBe('Fetched ahead; encodes when a slot frees');
		expect(stateLabel({ ...prefetched, progress: 0.5 })).toBe('Fetching source');
	});
});
