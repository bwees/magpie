import { describe, expect, it, vi } from 'vitest';
import { toast } from 'svelte-sonner';
import type { AttentionItemResponse, PipelineRowResponse } from '$lib/sdk';
import { LiveEventType, Projection } from '$lib/activity/activity-source';
import { AttentionAction, AttentionKind } from '$lib/activity/attention';
import { AttentionList } from '$lib/activity/attention-list.svelte';
import type { ActivityFeeds } from '$lib/activity/feeds';
import { LivePipeline } from '$lib/activity/live-pipeline.svelte';
import { Stage, TranscodeState, transcodeChipText } from '$lib/activity/pipeline';
import { JobState } from '$lib/transcode/gpu-jobs';
import { TitleDirectory } from '$lib/catalog/title-directory.svelte';
import { LoadStatus } from '$lib/load-status';
import { coalesce } from '$lib/live/coalesce';
import { FakeLive, settle } from '$lib/live/fake-live';
import type { RequestsApi } from '$lib/requests/requests-api';
import { RequestEvent, RequestStatus } from '$lib/requests/requests';
import { attentionItem, issue, media, pipelineRow, request, title } from '$lib/testing/fixtures';
import type { IssuesApi } from '$lib/requests/issues-api';
import { IssueAction, IssueProblem } from '$lib/requests/issues';
import { MyRequests } from './my-requests.svelte';
import { AGENTS_PATH, CONNECTIONS_PATH, NeedsYou, type NeedsYouApi } from './needs-you.svelte';
import { RecentlyActive } from './recently-active.svelte';
import { Trending } from './trending.svelte';

vi.mock('svelte-sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const HALF = 0.5;
const HALF_PERCENT = 50;
const ME = 'U1';

function feeds(overrides: Partial<ActivityFeeds> = {}): ActivityFeeds {
	return {
		attention: vi.fn(async () => []),
		active: vi.fn(async () => []),
		recent: vi.fn(async () => []),
		stages: vi.fn(async () => []),
		...overrides
	};
}

function issuesApi(overrides: Partial<IssuesApi> = {}): IssuesApi {
	return {
		list: vi.fn(async () => ({ items: [], total: 0 })),
		thread: vi.fn(async () => ({ issue: issue(), comments: [] })),
		report: vi.fn(async () => issue()),
		comment: vi.fn(async () => ({
			id: 'C1',
			author: { id: 'U1', displayName: 'Ana' },
			body: '',
			createdAt: ''
		})),
		act: vi.fn(async () => issue()),
		resolve: vi.fn(async () => issue()),
		removeWaitingImport: vi.fn(async () => undefined),
		...overrides
	};
}

function deferred<T>() {
	let resolve!: (value: T) => void;
	let reject!: (error: unknown) => void;
	const promise = new Promise<T>((res, rej) => {
		resolve = res;
		reject = rej;
	});
	return { promise, resolve, reject };
}

describe('coalesce', () => {
	it('runs once more after a burst instead of once per call', async () => {
		const gate = deferred<void>();
		const run = vi.fn(() => gate.promise);
		const refetch = coalesce(run);
		const first = refetch();
		void refetch();
		void refetch();
		gate.resolve();
		await first;
		await settle();
		expect(run).toHaveBeenCalledTimes(2);
	});
});

describe('Needs you', () => {
	it('shows a skeleton, then the items, and the empty state when nothing is open', async () => {
		const gate = deferred<AttentionItemResponse[]>();
		const list = new AttentionList(feeds({ attention: () => gate.promise }));
		const loading = list.load();
		expect(list.block.status).toBe(LoadStatus.LOADING);
		gate.resolve([]);
		await loading;
		expect(list.block.empty).toBe(true);
	});

	it('shows an inline error with Retry and keeps the shown items when a refetch fails', async () => {
		const attention = vi.fn<ActivityFeeds['attention']>().mockRejectedValueOnce(new Error('down'));
		const list = new AttentionList(feeds({ attention }));
		await list.load();
		expect(list.block.status).toBe(LoadStatus.ERROR);

		attention.mockResolvedValueOnce([attentionItem()]);
		await list.block.load();
		expect(list.count).toBe(1);

		attention.mockRejectedValueOnce(new Error('down'));
		await list.block.refresh();
		expect(list.block.status).toBe(LoadStatus.READY);
		expect(list.count).toBe(1);
	});

	it('refetches when an attention item opens or clears', async () => {
		const attention = vi.fn<ActivityFeeds['attention']>().mockResolvedValue([]);
		const list = new AttentionList(feeds({ attention }));
		const live = new FakeLive();
		await list.load();
		const stop = list.watch(live);

		attention.mockResolvedValue([attentionItem()]);
		live.emit(LiveEventType.ATTENTION_CHANGED, { item: 'A1', open: true, count: 1 });
		await settle();
		expect(list.count).toBe(1);
		stop();
		expect(live.listeners).toBe(0);
	});

	function needsYou(items: AttentionItemResponse[], api: Partial<NeedsYouApi> = {}) {
		const attention = vi.fn<ActivityFeeds['attention']>().mockResolvedValue(items);
		const list = new AttentionList(feeds({ attention }));
		const directory = new TitleDirectory(async (ids) =>
			ids.map((id) => title({ id, name: `Title ${id}` }))
		);
		const fullApi: NeedsYouApi = {
			approve: vi.fn(async () => ({})),
			searchAgain: vi.fn(async () => {}),
			request: vi.fn(async () => request({ kind: 'MOVIE', tmdbId: 7 })),
			media: vi.fn(async () => media({ name: 'Conclave' })),
			clear: vi.fn(async () => undefined),
			...api
		};
		return { list, attention, api: fullApi, block: new NeedsYou(list, directory, fullApi) };
	}

	it('names each row by its title, or by the pending request it is about', async () => {
		const { list, block } = needsYou([
			attentionItem(),
			attentionItem({
				id: 'A2',
				kind: AttentionKind.REQUEST_PENDING,
				ref: 'R1',
				titleId: null,
				action: AttentionAction.APPROVE
			}),
			attentionItem({
				id: 'A3',
				kind: AttentionKind.AGENT_OFFLINE,
				ref: 'G1',
				titleId: null,
				action: AttentionAction.VIEW_AGENT
			})
		]);
		await list.load();
		await block.resolve();
		expect(block.items.map((item) => block.subject(item))).toEqual([
			'Title T1',
			'Conclave',
			'Transcode agent'
		]);
	});

	it('approves through Requests and refetches the list', async () => {
		const item = attentionItem({
			kind: AttentionKind.REQUEST_PENDING,
			ref: 'R1',
			titleId: null,
			action: AttentionAction.APPROVE
		});
		const { list, block, api, attention } = needsYou([item]);
		await list.load();
		const button = block.button(item);
		expect(button.label).toBe('Approve');
		await button.run?.();
		expect(api.approve).toHaveBeenCalledWith('R1');
		expect(attention).toHaveBeenCalledTimes(2);
		expect(block.busy.size).toBe(0);
	});

	it('offers Clear only once items are shown', async () => {
		const { list, block } = needsYou([attentionItem()]);
		expect(block.canClear).toBe(false);
		await list.load();
		expect(block.canClear).toBe(true);
	});

	it('clears every item through Activity and empties the list', async () => {
		const { list, block, api, attention } = needsYou([attentionItem()]);
		await list.load();
		attention.mockResolvedValue([]);

		await block.clear();

		expect(api.clear).toHaveBeenCalledOnce();
		expect(list.count).toBe(0);
		expect(block.canClear).toBe(false);
		expect(block.clearing).toBe(false);
	});

	it('keeps the items and shows an error when Clear fails', async () => {
		const { list, block } = needsYou([attentionItem()], {
			clear: vi.fn(async () => Promise.reject(new Error('down')))
		});
		await list.load();

		await block.clear();

		expect(toast.error).toHaveBeenCalled();
		expect(list.count).toBe(1);
		expect(block.clearing).toBe(false);
	});

	it('searches again through Acquisition for a title with no accepted release', async () => {
		const item = attentionItem();
		const { list, block, api } = needsYou([item]);
		await list.load();
		await block.resolve();
		await block.button(item).run?.();
		expect(api.searchAgain).toHaveBeenCalledWith('T1', 'Title T1');
	});

	it('links agent, connection and title items to where they are fixed', () => {
		const { block } = needsYou([]);
		expect(block.button(attentionItem({ action: AttentionAction.VIEW_AGENT })).href).toBe(
			AGENTS_PATH
		);
		expect(block.button(attentionItem({ action: AttentionAction.OPEN_SETTINGS })).href).toBe(
			CONNECTIONS_PATH
		);
		expect(block.button(attentionItem({ action: AttentionAction.OPEN_TITLE })).href).toBe(
			'/titles/T1'
		);
	});

	it("re-grabs an issue's file through Requests and refetches the list", async () => {
		const item = attentionItem({
			kind: AttentionKind.ISSUE_OPEN,
			ref: 'I1',
			action: AttentionAction.REGRAB
		});
		const act = vi.fn(async () => issue());
		const { list, attention } = needsYou([item]);
		const block = new NeedsYou(
			list,
			new TitleDirectory(async () => []),
			undefined,
			issuesApi({ act })
		);
		await list.load();

		const button = block.button(item);
		expect(button.label).toBe('Blocklist & re-grab');
		await button.run?.();

		expect(act).toHaveBeenCalledWith('I1', IssueAction.REGRAB);
		expect(attention).toHaveBeenCalledTimes(2);
		expect(block.busy.size).toBe(0);
	});

	it('holds a re-grab back while a download waits for mapping', async () => {
		const item = attentionItem({
			kind: AttentionKind.ISSUE_OPEN,
			ref: 'I1',
			action: AttentionAction.REGRAB
		});
		const waiting = Object.assign(new Error('conflict'), {
			status: 409,
			data: {
				type: IssueProblem.IMPORT_WAITING,
				detail: 'X is waiting',
				instance: '/api/library/imports/D1'
			}
		});
		const { list } = needsYou([item]);
		const block = new NeedsYou(
			list,
			new TitleDirectory(async () => []),
			undefined,
			issuesApi({ act: vi.fn(async () => Promise.reject(waiting)) })
		);
		await list.load();

		await block.button(item).run?.();

		expect(block.fix.held?.download.downloadId).toBe('D1');
	});
});

describe('Live now', () => {
	const downloading = pipelineRow({ stage: Stage.DOWNLOADING, downloadId: 'D1' });
	const transcoding = pipelineRow({
		titleId: 'T2',
		stage: Stage.AVAILABLE,
		transcode: { state: TranscodeState.RUNNING, jobId: 'J1', reason: '', savedPercent: null }
	});

	it('overlays download and transcode percents from progress events without refetching', async () => {
		const active = vi.fn(async () => [downloading, transcoding]);
		const pipeline = new LivePipeline(feeds({ active }));
		const live = new FakeLive();
		await pipeline.load();
		pipeline.watch(live);

		live.emit(LiveEventType.DOWNLOAD_PROGRESSED, { download: 'D1', progress: HALF });
		live.emit(LiveEventType.JOB_PROGRESSED, {
			job: 'J1',
			stage: JobState.ENCODING,
			progress: HALF
		});
		live.emit(LiveEventType.DOWNLOAD_PROGRESSED, { download: 'OTHER', progress: HALF });

		expect(pipeline.progress.of(downloading)).toBe(HALF_PERCENT);
		expect(pipeline.progress.of(transcoding)).toBe(HALF_PERCENT);
		expect(active).toHaveBeenCalledTimes(1);
		expect(pipeline.count).toBe(2);
	});

	it('restarts the percent with the check once the encode is done', async () => {
		const pipeline = new LivePipeline(feeds({ active: vi.fn(async () => [transcoding]) }));
		const live = new FakeLive();
		await pipeline.load();
		pipeline.watch(live);

		live.emit(LiveEventType.JOB_PROGRESSED, { job: 'J1', stage: JobState.ENCODING, progress: 1 });
		live.emit(LiveEventType.JOB_PROGRESSED, {
			job: 'J1',
			stage: JobState.CHECKING,
			progress: HALF
		});

		const job = pipeline.progress.job(transcoding);
		expect(job).toEqual({ state: JobState.CHECKING, percent: HALF_PERCENT });
		expect(transcodeChipText(transcoding.transcode!, job)).toBe('Transcode check 50%');
	});

	it('refetches when the pipeline projection changes, and shows the empty state', async () => {
		const active = vi.fn<() => Promise<PipelineRowResponse[]>>().mockResolvedValue([downloading]);
		const pipeline = new LivePipeline(feeds({ active }));
		const live = new FakeLive();
		await pipeline.load();
		pipeline.watch(live);

		active.mockResolvedValue([]);
		live.emit(LiveEventType.PROJECTION_UPDATED, { projection: Projection.TIMELINE, title: 'T1' });
		await settle();
		expect(active).toHaveBeenCalledTimes(1);
		live.emit(LiveEventType.PROJECTION_UPDATED, { projection: Projection.PIPELINE, title: 'T1' });
		await settle();
		expect(pipeline.block.empty).toBe(true);
	});
});

describe('Recently active', () => {
	it('loads the recent rows with their titles', async () => {
		const byId = vi.fn(async (ids: string[]) => ids.map((id) => title({ id })));
		const recent = new RecentlyActive(
			new TitleDirectory(byId),
			feeds({
				recent: async () => [pipelineRow({ titleId: 'T1' }), pipelineRow({ titleId: 'T2' })]
			})
		);
		await recent.load();
		expect(recent.rows).toHaveLength(2);
		expect(recent.directory.get('T2')?.id).toBe('T2');
		expect(byId).toHaveBeenCalledWith(['T1', 'T2']);
	});

	it('shows an inline error on failure and recovers on Retry', async () => {
		const recentFeed = vi.fn<ActivityFeeds['recent']>().mockRejectedValueOnce(new Error('down'));
		const recent = new RecentlyActive(
			new TitleDirectory(async () => []),
			feeds({ recent: recentFeed })
		);
		await recent.load();
		expect(recent.block.status).toBe(LoadStatus.ERROR);
		recentFeed.mockResolvedValueOnce([]);
		await recent.block.load();
		expect(recent.block.empty).toBe(true);
	});
});

describe('Requester home', () => {
	function requestsApi(items = [request()]): RequestsApi {
		return {
			list: vi.fn(async () => ({ items, counts: [], total: items.length })),
			create: vi.fn(),
			approve: vi.fn(),
			decline: vi.fn(),
			retry: vi.fn(),
			cancel: vi.fn(),
			defaultProfile: vi.fn(),
			media: vi.fn(async () => media({ name: 'Dune' }))
		};
	}

	it('lists only the viewer’s requests, with the stage while they are on their way', async () => {
		const mine = request({ id: 'R1', status: RequestStatus.PROCESSING, titleId: 'T1' });
		const pending = request({ id: 'R2', status: RequestStatus.PENDING });
		const theirs = request({ id: 'R3', requester: { id: 'U2', displayName: 'Bo' } });
		const home = new MyRequests(
			ME,
			requestsApi([mine, pending, theirs]),
			feeds({ stages: async () => [pipelineRow({ stage: Stage.DOWNLOADING, detail: 'Upgrade' })] })
		);
		await home.load();
		expect(home.items.map((r) => r.id)).toEqual(['R1', 'R2']);
		expect(home.badge(mine)).toEqual({ label: 'Downloading', failed: false, detail: 'Upgrade' });
		expect(home.badge(pending).label).toBe('Pending');
		expect(home.nameOf(mine)).toBe('Dune');
	});

	it('refetches a stage on a pipeline update and the list on request events', async () => {
		const mine = request({ status: RequestStatus.APPROVED, titleId: 'T1' });
		const api = requestsApi([mine]);
		const stages = vi
			.fn<ActivityFeeds['stages']>()
			.mockResolvedValue([pipelineRow({ stage: Stage.SEARCHING })]);
		const home = new MyRequests(ME, api, feeds({ stages }));
		const live = new FakeLive();
		await home.load();
		home.watch(live);

		stages.mockResolvedValue([pipelineRow({ stage: Stage.IMPORTING })]);
		live.emit(LiveEventType.PROJECTION_UPDATED, { projection: Projection.PIPELINE, title: 'T1' });
		await settle();
		expect(home.badge(mine).label).toBe('Importing');

		live.emit(RequestEvent.APPROVED, { request: 'R1' });
		await settle();
		expect(api.list).toHaveBeenCalledTimes(2);
	});

	it('shows the error state when the requests can’t load', async () => {
		const api = requestsApi();
		api.list = vi.fn(async () => Promise.reject(new Error('down')));
		const home = new MyRequests(ME, api, feeds());
		await home.load();
		expect(home.block.status).toBe(LoadStatus.ERROR);
	});

	it('shows the trending row, or its error with Retry', async () => {
		const source = vi
			.fn()
			.mockRejectedValueOnce(new Error('down'))
			.mockResolvedValueOnce([media()]);
		const trending = new Trending(source);
		await trending.load();
		expect(trending.block.status).toBe(LoadStatus.ERROR);
		await trending.load();
		expect(trending.items).toHaveLength(1);
	});
});
