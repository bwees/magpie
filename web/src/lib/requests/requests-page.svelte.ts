import { toast } from 'svelte-sonner';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import {
	apiErrorMessage,
	type DiscoverDetailResponse,
	type RequestResponse,
	type StatusCountResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import type { LiveEvents } from '$lib/activity/activity-source';
import { SDK_REQUESTS_API, type RequestsApi } from './requests-api';
import {
	FILTER_STATUSES,
	ITEM_AVAILABLE_EVENT,
	ITEM_AVAILABLE_SETTLE_MS,
	REQUEST_EVENTS,
	RequestStatus,
	StatusFilter,
	filterCount,
	mediaKey
} from './requests';

export interface Viewer {
	id: string;
	admin: boolean;
}

/**
 * The Requests page: the status chips with their counts, the rows, and the approve, decline, retry and
 * cancel actions. Every request event refetches, so the counts stay live.
 */
export class RequestsPage {
	filter = $state(StatusFilter.PENDING);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	items = $state<RequestResponse[]>([]);
	counts = $state<StatusCountResponse[]>([]);
	/** Name, year and poster per movie or series, keyed by mediaKey. */
	readonly media = new SvelteMap<string, DiscoverDetailResponse>();
	/** Requests with an action in flight. */
	readonly busy = new SvelteSet<string>();

	approving = $state<RequestResponse>();
	approveProfile = $state<string>();
	declining = $state<RequestResponse>();
	declineReason = $state('');

	readonly viewer: Viewer;
	readonly #api: RequestsApi;
	#latest: Promise<unknown> | undefined;

	constructor(viewer: Viewer, api: RequestsApi = SDK_REQUESTS_API) {
		this.viewer = viewer;
		this.#api = api;
		this.filter = viewer.admin ? StatusFilter.PENDING : StatusFilter.ALL;
	}

	get empty(): boolean {
		return this.status === LoadStatus.READY && this.items.length === 0;
	}

	count(filter: StatusFilter): number {
		return filterCount(this.counts, filter);
	}

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		await this.refresh();
	}

	select(filter: StatusFilter): Promise<void> {
		this.filter = filter;
		return this.load();
	}

	/**
	 * Refetches the rows; the newest call wins when several overlap. A failed load shows the error; a
	 * failed refetch keeps the rows shown.
	 */
	async refresh(): Promise<void> {
		const request = this.#api.list(FILTER_STATUSES[this.filter]);
		this.#latest = request;
		try {
			const result = await request;
			if (this.#latest !== request) return;
			this.items = result.items;
			this.counts = result.counts;
			this.error = '';
			this.status = LoadStatus.READY;
			void this.#loadMedia(result.items);
		} catch (error) {
			if (this.#latest !== request) return;
			this.error = apiErrorMessage(error, 'Could not load requests.');
			if (this.status === LoadStatus.LOADING || this.items.length === 0) {
				this.status = LoadStatus.ERROR;
			}
		}
	}

	async #loadMedia(items: RequestResponse[]): Promise<void> {
		const keys = items.map((item) => mediaKey(item.kind, item.tmdbId));
		const missing = items.filter(
			(_, index) => !this.media.has(keys[index]) && keys.indexOf(keys[index]) === index
		);
		await Promise.all(
			missing.map(async (item) => {
				try {
					this.media.set(
						mediaKey(item.kind, item.tmdbId),
						await this.#api.media(item.kind, item.tmdbId)
					);
				} catch {
					// The row falls back to the TMDB ID.
				}
			})
		);
	}

	mediaOf(request: RequestResponse): DiscoverDetailResponse | undefined {
		return this.media.get(mediaKey(request.kind, request.tmdbId));
	}

	canDecide(request: RequestResponse): boolean {
		return this.viewer.admin && request.status === RequestStatus.PENDING;
	}

	canRetry(request: RequestResponse): boolean {
		return this.viewer.admin && request.status === RequestStatus.FAILED;
	}

	canCancel(request: RequestResponse): boolean {
		return request.requester.id === this.viewer.id && request.status === RequestStatus.PENDING;
	}

	/** Opens the approve dialog with the default profile picked. */
	async askToApprove(request: RequestResponse): Promise<void> {
		this.approving = request;
		this.approveProfile = request.profileId;
		try {
			this.approveProfile = await this.#api.defaultProfile();
		} catch {
			// Keep the profile the request was created with.
		}
	}

	askToDecline(request: RequestResponse): void {
		this.declining = request;
		this.declineReason = '';
	}

	async approve(): Promise<void> {
		const request = this.approving;
		if (!request) return;
		const done = await this.#act(
			request,
			() => this.#api.approve(request.id, this.approveProfile),
			'Approved'
		);
		if (done) this.approving = undefined;
	}

	async decline(): Promise<void> {
		const request = this.declining;
		if (!request) return;
		const done = await this.#act(
			request,
			() => this.#api.decline(request.id, this.declineReason.trim()),
			'Declined'
		);
		if (done) this.declining = undefined;
	}

	retry(request: RequestResponse): Promise<boolean> {
		return this.#act(request, () => this.#api.retry(request.id), 'Approved again');
	}

	cancel(request: RequestResponse): Promise<boolean> {
		return this.#act(request, () => this.#api.cancel(request.id), 'Request cancelled');
	}

	async #act(
		request: RequestResponse,
		action: () => Promise<unknown>,
		done: string
	): Promise<boolean> {
		this.busy.add(request.id);
		try {
			await action();
			toast.success(`${done}: ${this.titleOf(request)}`);
			await this.refresh();
			return true;
		} catch (error) {
			toast.error(apiErrorMessage(error));
			return false;
		} finally {
			this.busy.delete(request.id);
		}
	}

	titleOf(request: RequestResponse): string {
		return this.mediaOf(request)?.name ?? `TMDB ${request.tmdbId}`;
	}

	/** Follows request and availability events until the returned function is called. */
	watch(live: LiveEvents): () => void {
		let settle: ReturnType<typeof setTimeout> | undefined;
		const stops = REQUEST_EVENTS.map((type) => live.on(type, () => void this.refresh()));
		stops.push(
			live.on(ITEM_AVAILABLE_EVENT, () => {
				clearTimeout(settle);
				settle = setTimeout(() => void this.refresh(), ITEM_AVAILABLE_SETTLE_MS);
			}),
			() => clearTimeout(settle)
		);
		stops.push(live.onReset(() => void this.refresh()));
		return () => stops.forEach((stop) => stop());
	}
}
