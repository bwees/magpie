import { beforeEach, describe, expect, it, vi } from 'vitest';
import { toast } from 'svelte-sonner';
import { HttpError } from '@oazapfts/runtime';
import type { LiveHandler, ResetListener } from '$lib/live/live-connection';
import { searchTopic, type LiveMessage } from '$lib/live/protocol';
import type {
	GrabRequest,
	GrabResponse,
	ReleaseDecision,
	ReleaseGrabbed,
	ReleaseRejected,
	SearchIndexerResponse,
	SearchResponse,
	SearchResultsReceived
} from '$lib/sdk';
import {
	DropReason,
	GrabState,
	IndexerStatus,
	InteractiveSearch,
	SearchEventType,
	SearchPhase,
	type SearchApi,
	type SearchLive
} from './interactive-search.svelte';

vi.mock('svelte-sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const TITLE = '01TITLE';
const SEARCH = '01SEARCH';
const WEB_1080P = 3;
const WEB_720P = 5;
const BAD_GATEWAY = 502;
const DOWNLOAD = '01DOWNLOAD';
const NEXT_DOWNLOAD = '01NEXT';

function grabbedEvent(r: ReleaseDecision, download: string): ReleaseGrabbed {
	return {
		download,
		title: TITLE,
		episodes: [],
		release: r.name,
		indexer: r.indexer,
		infoHash: '',
		quality: r.quality,
		cfScore: r.cfScore,
		trigger: 'INTERACTIVE',
		override: false
	};
}

function release(id: string, rank: number, change: Partial<ReleaseDecision> = {}): ReleaseDecision {
	return {
		release: id,
		name: `Movie.Title.2024.1080p.WEB-DL-${id}`,
		indexer: 'Nyaa',
		infoHash: '',
		quality: { id: WEB_1080P, name: 'WEBDL-1080p', source: 'WEBDL', resolution: 1080, revision: 1 },
		formats: [],
		cfScore: 0,
		sizeBytes: 1024 ** 3,
		seeders: 10,
		publishedAt: '2026-10-11T12:00:00Z',
		freeleech: false,
		rejections: [],
		rank,
		...change
	};
}

function rejected(id: string): ReleaseDecision {
	return release(id, 0, {
		quality: { id: WEB_720P, name: 'WEBDL-720p', source: 'WEBDL', resolution: 720, revision: 1 },
		rejections: [
			{ reason: 'QUALITY_NOT_ALLOWED', message: "WEBDL-720p isn't wanted", temporary: false }
		]
	});
}

function indexer(name: string, status: IndexerStatus): SearchIndexerResponse {
	return { name, status, message: '' };
}

function snapshot(change: Partial<SearchResponse> = {}): SearchResponse {
	return {
		searchId: SEARCH,
		title: TITLE,
		trigger: 'INTERACTIVE',
		startedAt: '2026-10-11T12:00:00Z',
		completed: false,
		cancelled: false,
		profile: {
			id: 'p1',
			name: '1080p',
			cutoff: '1080p',
			buckets: [{ name: '1080p', qualityIds: [WEB_1080P], meetsCutoff: true }],
			penalizedFormats: ['LQ']
		},
		indexers: [indexer('Nyaa', IndexerStatus.PENDING), indexer('Beyond', IndexerStatus.PENDING)],
		releases: [],
		...change
	};
}

class FakeLive implements SearchLive {
	readonly handlers = new Map<string, Set<LiveHandler>>();
	readonly resets = new Set<ResetListener>();
	readonly topics = new Set<string>();

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

	subscribe(topic: string): void {
		this.topics.add(topic);
	}

	unsubscribe(topic: string): void {
		this.topics.delete(topic);
	}

	emit(type: string, data: unknown): void {
		const message: LiveMessage = { seq: 1, epoch: 'E', type, topic: searchTopic(SEARCH), data };
		this.handlers.get(type)?.forEach((handler) => handler(message));
	}

	results(indexerName: string, status: IndexerStatus, releases: ReleaseDecision[]): void {
		const event: SearchResultsReceived = {
			search: SEARCH,
			indexer: indexerName,
			status,
			message: '',
			releases
		};
		this.emit(SearchEventType.RESULTS, event);
	}
}

class FakeApi implements SearchApi {
	started = snapshot();
	current = snapshot();
	grabs: GrabRequest[] = [];
	cancelled: string[] = [];

	start = vi.fn(async () => this.started);
	get = vi.fn(async () => this.current);

	async cancel(id: string): Promise<unknown> {
		this.cancelled.push(id);
		return undefined;
	}

	/** The answer to the next grab, or an error to throw. */
	answer: GrabResponse | HttpError = { status: GrabState.CHECKING, download: DOWNLOAD };

	async grab(_id: string, request: GrabRequest): Promise<GrabResponse> {
		this.grabs.push(request);
		if (this.answer instanceof HttpError) {
			throw this.answer;
		}
		return this.answer;
	}
}

describe('InteractiveSearch', () => {
	let live: FakeLive;
	let api: FakeApi;
	let search: InteractiveSearch;

	beforeEach(() => {
		vi.clearAllMocks();
		live = new FakeLive();
		api = new FakeApi();
		search = new InteractiveSearch(TITLE, live, api);
	});

	it('starts the search, subscribes to its topic and reads the cached state', async () => {
		api.current = snapshot({ releases: [release('a', 1)] });

		await search.start();

		expect(api.start).toHaveBeenCalledWith(TITLE);
		expect(live.topics.has(searchTopic(SEARCH))).toBe(true);
		expect(search.phase).toBe(SearchPhase.RUNNING);
		expect(search.indexers.map((i) => i.status)).toEqual([
			IndexerStatus.PENDING,
			IndexerStatus.PENDING
		]);
		expect(search.releases.map((r) => r.release)).toEqual(['a']);
	});

	it('appends streamed releases to their bucket without reordering and stars the best', async () => {
		await search.start();

		live.results('Nyaa', IndexerStatus.DONE, [release('a', 1), rejected('r')]);
		live.results('Beyond', IndexerStatus.DONE, [release('b', 1)]);

		const [bucket] = search.buckets;
		expect(bucket.releases.map((r) => r.release)).toEqual(['a', 'b']);
		expect(bucket.best).toBe('b');
		expect(search.order).toEqual(['b', 'a']);
		expect(search.rejected.map((r) => r.release)).toEqual(['r']);
		expect(search.answered).toBe(2);
		expect(search.isPenalized('LQ')).toBe(true);
	});

	it('takes a late answer after a timeout but never moves an indexer back', async () => {
		await search.start();

		live.results('Nyaa', IndexerStatus.FAILED, []);
		expect(search.failures).toBe(1);
		live.results('Nyaa', IndexerStatus.DONE, [release('late', 1)]);
		live.results('Nyaa', IndexerStatus.FAILED, []);

		expect(search.indexers[0].status).toBe(IndexerStatus.DONE);
		expect(search.releases.map((r) => r.release)).toEqual(['late']);
	});

	it('ignores other searches and finishes on SearchCompleted', async () => {
		await search.start();

		live.emit(SearchEventType.RESULTS, {
			search: 'other',
			indexer: 'Nyaa',
			status: IndexerStatus.DONE,
			message: '',
			releases: [release('x', 1)]
		});
		live.results('Nyaa', IndexerStatus.FAILED, []);
		live.results('Beyond', IndexerStatus.FAILED, []);
		live.emit(SearchEventType.COMPLETED, { search: SEARCH, failedIndexers: ['Nyaa', 'Beyond'] });

		expect(search.releases).toEqual([]);
		expect(search.phase).toBe(SearchPhase.FINISHED);
		expect(search.allFailed).toBe(true);
	});

	it('refetches the cached state when the live socket resets', async () => {
		await search.start();
		api.current = snapshot({
			completed: true,
			indexers: [indexer('Nyaa', IndexerStatus.DONE), indexer('Beyond', IndexerStatus.SKIPPED)],
			releases: [release('a', 2), release('b', 1)]
		});

		live.resets.forEach((reset) => reset());
		await vi.waitFor(() => expect(search.completed).toBe(true));

		expect(search.order).toEqual(['b', 'a']);
		expect(search.buckets[0].best).toBe('b');
	});

	it('sends override for a rejected release and shows why a grab failed', async () => {
		await search.start();
		live.results('Nyaa', IndexerStatus.DONE, [release('a', 1), rejected('r')]);
		api.answer = new HttpError(
			BAD_GATEWAY,
			{ detail: 'qBittorrent is unreachable' },
			new Headers()
		);

		await search.grab(search.accepted[0]);
		await search.grab(search.rejected[0]);

		expect(api.grabs).toEqual([
			{ releaseId: 'a', override: false },
			{ releaseId: 'r', override: true }
		]);
		expect(toast.error).toHaveBeenCalledWith('qBittorrent is unreachable', expect.anything());
		expect(search.grabs.size).toBe(0);
	});

	it('checks a magnet, then shows it downloading once its metadata passed', async () => {
		await search.start();
		live.results('Nyaa', IndexerStatus.DONE, [release('a', 1)]);
		const [grabbed] = search.accepted;

		await search.grab(grabbed);
		expect(search.grabState(grabbed)).toBe(GrabState.CHECKING);

		live.emit(SearchEventType.PROGRESSED, { download: DOWNLOAD, state: 'AWAITING_METADATA' });
		expect(search.grabState(grabbed)).toBe(GrabState.CHECKING);
		live.emit(SearchEventType.PROGRESSED, { download: DOWNLOAD, state: 'DOWNLOADING' });
		expect(search.grabState(grabbed)).toBe(GrabState.DOWNLOADING);
	});

	it('shows a blocked release rejected once and marks the fallback grab', async () => {
		await search.start();
		live.results('Nyaa', IndexerStatus.DONE, [release('a', 2), release('b', 1)]);
		const [first, next] = search.accepted;
		api.answer = { status: GrabState.REJECTED, blockedFile: 'Movie.exe', next: NEXT_DOWNLOAD };

		const rejection: ReleaseRejected = {
			title: TITLE,
			release: first.name,
			indexer: first.indexer,
			infoHash: 'h',
			reason: DropReason.BLOCKED_FILE,
			blockedFile: 'Movie.exe',
			next: NEXT_DOWNLOAD
		};
		live.emit(SearchEventType.GRABBED, grabbedEvent(next, NEXT_DOWNLOAD));
		await search.grab(first);
		live.emit(SearchEventType.REJECTED, rejection);

		expect(search.grabState(first)).toBe(GrabState.REJECTED);
		expect(search.grabState(next)).toBe(GrabState.CHECKING);
		expect(toast.error).toHaveBeenCalledTimes(1);
		expect(toast.error).toHaveBeenCalledWith(
			'Rejected: Movie.exe is a blocked file type',
			expect.anything()
		);
	});

	it('rejects a checked magnet when its rejection arrives later', async () => {
		await search.start();
		live.results('Nyaa', IndexerStatus.DONE, [release('a', 1)]);
		const [grabbed] = search.accepted;
		await search.grab(grabbed);

		live.emit(SearchEventType.REJECTED, {
			title: TITLE,
			release: grabbed.name,
			indexer: grabbed.indexer,
			infoHash: 'h',
			reason: DropReason.METADATA_TIMEOUT,
			blockedFile: null,
			next: null
		} satisfies ReleaseRejected);

		expect(search.grabState(grabbed)).toBe(GrabState.REJECTED);
		expect(toast.error).toHaveBeenCalledWith(
			'Rejected: the file list never arrived',
			expect.objectContaining({ description: expect.stringContaining('searching again') })
		);
	});

	it('ignores grabs of other titles', async () => {
		await search.start();
		live.results('Nyaa', IndexerStatus.DONE, [release('a', 1)]);

		live.emit(SearchEventType.GRABBED, {
			...grabbedEvent(search.accepted[0], DOWNLOAD),
			title: 'other'
		});

		expect(search.grabs.size).toBe(0);
	});

	it('cancels a running search when it stops, but not a finished one', async () => {
		await search.start();
		search.stop();
		expect(api.cancelled).toEqual([SEARCH]);
		expect(live.topics.size).toBe(0);

		api.started = snapshot({ completed: true });
		api.current = api.started;
		await search.start();
		search.stop();
		expect(api.cancelled).toEqual([SEARCH]);
	});

	it('reports a search that could not start', async () => {
		api.start.mockRejectedValueOnce(new HttpError(404, { detail: 'No such title' }, new Headers()));

		await search.start();

		expect(search.phase).toBe(SearchPhase.FAILED);
		expect(search.error).toBe('No such title');
	});
});
