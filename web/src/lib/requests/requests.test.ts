import { beforeEach, describe, expect, it, vi } from 'vitest';
import type {
	CreateRequestRequest,
	DiscoverDetailResponse,
	DiscoverItemResponse,
	RequestListResponse,
	RequestResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import type { LiveHandler, ResetListener } from '$lib/live/live-connection';
import type { LiveMessage } from '$lib/live/protocol';
import type { RequestsApi } from './requests-api';
import { PendingRequests } from './pending-requests.svelte';
import { RequestsPage } from './requests-page.svelte';
import {
	RequestEvent,
	RequestProblem,
	RequestScope,
	RequestStatus,
	StatusFilter,
	conflictOf,
	filterCount,
	ITEM_AVAILABLE_EVENT,
	ITEM_AVAILABLE_SETTLE_MS,
	initials,
	scopeLabel,
	statusLabel
} from './requests';
import { CardState, SearchRequests } from './search-requests.svelte';
import { TitleRequester } from './title-requester.svelte';

const ADMIN = { id: 'admin', admin: true };
const ALICE = { id: 'alice', admin: false };
const HTTP_CONFLICT = 409;

function request(id: string, status: RequestStatus, user = ALICE.id, tmdbId = 1): RequestResponse {
	return {
		id,
		status,
		tmdbId,
		kind: 'MOVIE',
		scope: RequestScope.MOVIE,
		seasons: [],
		profileId: '1080p',
		requester: { id: user, displayName: user },
		titleId: null,
		createdAt: '2026-10-11T12:00:00Z',
		decidedAt: null,
		decidedBy: null,
		declineReason: '',
		failureReason: '',
		availableItems: 0,
		totalItems: 0
	};
}

function conflict(type: RequestProblem): Error {
	return Object.assign(new Error('conflict'), { status: HTTP_CONFLICT, data: { type } });
}

class FakeApi implements RequestsApi {
	rows: RequestResponse[] = [];
	listCalls: { statuses: string[]; count?: boolean; titleId?: string }[] = [];
	mediaCalls: number[] = [];
	created: CreateRequestRequest[] = [];
	approved: { id: string; profileId?: string }[] = [];
	declined: { id: string; reason: string }[] = [];
	createResult: () => Promise<RequestResponse> = async () => request('new', RequestStatus.PENDING);

	async list(
		statuses: string[],
		options: { count?: boolean; titleId?: string } = {}
	): Promise<RequestListResponse> {
		this.listCalls.push({ statuses, ...options });
		const items = this.rows.filter((r) => statuses.length === 0 || statuses.includes(r.status));
		const counts = Object.values(RequestStatus).map((status) => ({
			status,
			count: this.rows.filter((r) => r.status === status).length
		}));
		return { items: options.count ? [] : items, total: items.length, counts };
	}

	create(body: CreateRequestRequest): Promise<RequestResponse> {
		this.created.push(body);
		return this.createResult();
	}

	async approve(id: string, profileId?: string): Promise<RequestResponse> {
		this.approved.push({ id, profileId });
		this.#set(id, RequestStatus.APPROVED);
		return request(id, RequestStatus.APPROVED);
	}

	async decline(id: string, reason: string): Promise<RequestResponse> {
		this.declined.push({ id, reason });
		this.#set(id, RequestStatus.DECLINED);
		return request(id, RequestStatus.DECLINED);
	}

	async retry(id: string): Promise<RequestResponse> {
		this.#set(id, RequestStatus.APPROVED);
		return request(id, RequestStatus.APPROVED);
	}

	async cancel(id: string): Promise<unknown> {
		this.rows = this.rows.filter((r) => r.id !== id);
		return undefined;
	}

	async defaultProfile(): Promise<string> {
		return '2160p';
	}

	async media(kind: string, tmdbId: number): Promise<DiscoverDetailResponse> {
		this.mediaCalls.push(tmdbId);
		return {
			kind,
			tmdbId,
			name: `Title ${tmdbId}`,
			year: 2024,
			overview: '',
			posterUrl: null,
			inLibrary: false,
			titleId: null,
			seasons: [
				{ number: 0, episodeCount: 1, monitored: false },
				{ number: 1, episodeCount: 8, monitored: true },
				{ number: 2, episodeCount: 8, monitored: false }
			]
		};
	}

	#set(id: string, status: RequestStatus): void {
		this.rows = this.rows.map((r) => (r.id === id ? { ...r, status } : r));
	}
}

class FakeLive {
	readonly handlers = new Map<string, LiveHandler[]>();
	readonly resets: ResetListener[] = [];

	on(type: string, handler: LiveHandler): () => void {
		this.handlers.set(type, [...(this.handlers.get(type) ?? []), handler]);
		return () =>
			this.handlers.set(
				type,
				(this.handlers.get(type) ?? []).filter((h) => h !== handler)
			);
	}

	onReset(listener: ResetListener): () => void {
		this.resets.push(listener);
		return () => undefined;
	}

	emit(type: string): void {
		const message: LiveMessage = { seq: 1, epoch: 'e', type, topic: '', data: {} };
		for (const handler of this.handlers.get(type) ?? []) handler(message);
	}
}

const settle = () => new Promise((resolve) => setTimeout(resolve));

describe('request helpers', () => {
	it('counts a chip over its statuses, and every status for All', () => {
		const counts = [
			{ status: RequestStatus.PENDING, count: 2 },
			{ status: RequestStatus.PROCESSING, count: 3 },
			{ status: RequestStatus.FAILED, count: 1 },
			{ status: RequestStatus.DECLINED, count: 4 }
		];
		expect(filterCount(counts, StatusFilter.PENDING)).toBe(2);
		expect(filterCount(counts, StatusFilter.APPROVED)).toBe(4);
		expect(filterCount(counts, StatusFilter.ALL)).toBe(10);
	});

	it('labels scopes', () => {
		expect(scopeLabel(RequestScope.MOVIE, [])).toBe('Movie');
		expect(scopeLabel(RequestScope.ALL_SEASONS, [])).toBe('All seasons');
		expect(scopeLabel(RequestScope.SEASONS, [2])).toBe('Season 2');
		expect(scopeLabel(RequestScope.SEASONS, [1, 3])).toBe('Seasons 1, 3');
	});

	it('recognises request conflicts only', () => {
		expect(conflictOf(conflict(RequestProblem.ALREADY_REQUESTED))).toBe(
			RequestProblem.ALREADY_REQUESTED
		);
		expect(conflictOf(new Error('boom'))).toBeUndefined();
	});

	it('takes up to two initials', () => {
		expect(initials('Alice Liddell Smith')).toBe('AL');
		expect(initials('bob')).toBe('B');
	});
});

describe('RequestsPage', () => {
	let api: FakeApi;

	beforeEach(() => {
		api = new FakeApi();
		api.rows = [
			request('p1', RequestStatus.PENDING, ALICE.id, 1),
			request('p2', RequestStatus.PENDING, 'bob', 1),
			request('f1', RequestStatus.FAILED, 'bob', 2)
		];
	});

	it('starts admins on Pending and requesters on All, with counts per chip', async () => {
		const admin = new RequestsPage(ADMIN, api);
		await admin.load();
		expect(admin.filter).toBe(StatusFilter.PENDING);
		expect(admin.items.map((r) => r.id)).toEqual(['p1', 'p2']);
		expect(admin.count(StatusFilter.APPROVED)).toBe(1);

		const requester = new RequestsPage(ALICE, api);
		expect(requester.filter).toBe(StatusFilter.ALL);
	});

	it('loads each title once for its rows', async () => {
		const page = new RequestsPage(ADMIN, api);
		await page.select(StatusFilter.ALL);
		await settle();
		expect(api.mediaCalls.sort()).toEqual([1, 2]);
		expect(page.titleOf(api.rows[0])).toBe('Title 1');
	});

	it('approves with the default profile and refetches', async () => {
		const page = new RequestsPage(ADMIN, api);
		await page.load();
		await page.askToApprove(page.items[0]);
		expect(page.approveProfile).toBe('2160p');

		await page.approve();

		expect(api.approved).toEqual([{ id: 'p1', profileId: '2160p' }]);
		expect(page.approving).toBeUndefined();
		expect(page.items.map((r) => r.id)).toEqual(['p2']);
	});

	it('declines with the trimmed reason', async () => {
		const page = new RequestsPage(ADMIN, api);
		await page.load();
		page.askToDecline(page.items[1]);
		page.declineReason = '  not in 4K ';

		await page.decline();

		expect(api.declined).toEqual([{ id: 'p2', reason: 'not in 4K' }]);
		expect(page.count(StatusFilter.DECLINED)).toBe(1);
	});

	it('offers each action only where it applies', async () => {
		const admin = new RequestsPage(ADMIN, api);
		const alice = new RequestsPage(ALICE, api);
		const [mine, bobs, failed] = api.rows;

		expect(admin.canDecide(mine)).toBe(true);
		expect(alice.canDecide(mine)).toBe(false);
		expect(alice.canCancel(mine)).toBe(true);
		expect(alice.canCancel(bobs)).toBe(false);
		expect(admin.canRetry(failed)).toBe(true);
		expect(alice.canRetry(failed)).toBe(false);
	});

	it('refetches on request events', async () => {
		const page = new RequestsPage(ADMIN, api);
		const live = new FakeLive();
		await page.load();
		const stop = page.watch(live);

		api.rows.push(request('p3', RequestStatus.PENDING));
		live.emit(RequestEvent.CREATED);
		await settle();

		expect(page.items).toHaveLength(3);
		stop();
	});

	it('refetches once Requests has counted an item Jellyfin confirmed', async () => {
		const page = new RequestsPage(ADMIN, api);
		const live = new FakeLive();
		await page.select(StatusFilter.ALL);
		vi.useFakeTimers();
		const stop = page.watch(live);

		api.rows[0] = { ...api.rows[0], status: RequestStatus.PARTIALLY_AVAILABLE };
		live.emit(ITEM_AVAILABLE_EVENT);
		live.emit(ITEM_AVAILABLE_EVENT);
		const calls = api.listCalls.length;
		await vi.advanceTimersByTimeAsync(ITEM_AVAILABLE_SETTLE_MS);
		vi.useRealTimers();
		await settle();

		expect(api.listCalls.length).toBe(calls + 1);
		expect(page.items[0].status).toBe(RequestStatus.PARTIALLY_AVAILABLE);
		stop();
	});
});

describe('statusLabel', () => {
	it('shows the progress of a partly available request', () => {
		const partial = {
			...request('r1', RequestStatus.PARTIALLY_AVAILABLE),
			availableItems: 4,
			totalItems: 10
		};
		expect(statusLabel(partial)).toBe('4 of 10 available');
		expect(statusLabel(request('r2', RequestStatus.AVAILABLE))).toBe('Available');
	});
});

describe('PendingRequests', () => {
	it('counts pending requests and recounts on request events', async () => {
		const api = new FakeApi();
		api.rows = [request('p1', RequestStatus.PENDING)];
		const pending = new PendingRequests(api);
		const live = new FakeLive();
		await pending.load();
		pending.watch(live);
		expect(pending.count).toBe(1);
		expect(api.listCalls[0]).toEqual({ statuses: [RequestStatus.PENDING], count: true });

		api.rows = [];
		live.emit(RequestEvent.APPROVED);
		await settle();
		expect(pending.count).toBe(0);
	});
});

describe('SearchRequests', () => {
	const dune: DiscoverItemResponse = {
		kind: 'MOVIE',
		tmdbId: 1,
		name: 'Dune',
		year: 2024,
		overview: '',
		posterUrl: null,
		inLibrary: false,
		titleId: null
	};
	const severance: DiscoverItemResponse = { ...dune, kind: 'SERIES', tmdbId: 2, name: 'Severance' };

	it('marks open requests and library titles', async () => {
		const api = new FakeApi();
		api.rows = [request('p1', RequestStatus.PENDING, ALICE.id, 1)];
		const search = new SearchRequests(api);
		await search.load();

		expect(search.stateOf(dune)).toBe(CardState.REQUESTED);
		expect(search.stateOf(severance)).toBe(CardState.REQUESTABLE);
		expect(search.stateOf({ ...severance, inLibrary: true })).toBe(CardState.IN_LIBRARY);
	});

	it('requests a movie at once and marks it requested', async () => {
		const api = new FakeApi();
		const search = new SearchRequests(api);

		await search.request(dune);

		expect(api.created).toEqual([{ kind: 'MOVIE', tmdbId: 1, scope: RequestScope.MOVIE }]);
		expect(search.stateOf(dune)).toBe(CardState.REQUESTED);
	});

	it('marks a card requested when someone else already asked for it', async () => {
		const api = new FakeApi();
		api.createResult = () => Promise.reject(conflict(RequestProblem.ALREADY_REQUESTED));
		const search = new SearchRequests(api);

		await search.request(dune);

		expect(search.stateOf(dune)).toBe(CardState.REQUESTED);
	});

	it('opens the scope picker for a series and sends the chosen seasons', async () => {
		const api = new FakeApi();
		const search = new SearchRequests(api);

		await search.request(severance);
		const picker = search.picker!;
		expect(picker.status).toBe(LoadStatus.READY);
		expect(picker.body()).toEqual({ kind: 'SERIES', tmdbId: 2, scope: RequestScope.ALL_SEASONS });

		picker.toggle(2, true);
		picker.toggle(0, true);
		await search.submitPicker();

		expect(api.created).toEqual([
			{ kind: 'SERIES', tmdbId: 2, scope: RequestScope.SEASONS, seasons: [0, 2] }
		]);
		expect(search.picker).toBeUndefined();
	});

	it('cannot send a seasons request without a season', async () => {
		const api = new FakeApi();
		const search = new SearchRequests(api);
		await search.request(severance);
		search.picker!.toggle(1, false);

		await search.submitPicker();

		expect(search.picker!.canSubmit).toBe(false);
		expect(api.created).toEqual([]);
	});
});

describe('TitleRequester', () => {
	it('names the first requester of a Title', async () => {
		const api = new FakeApi();
		api.rows = [
			request('newer', RequestStatus.PROCESSING, 'bob'),
			request('older', RequestStatus.AVAILABLE, 'alice')
		];
		const requester = new TitleRequester(api);

		await requester.load('title-1');

		expect(requester.name).toBe('alice');
		expect(api.listCalls[0]).toEqual({ statuses: [], titleId: 'title-1' });
	});
});
