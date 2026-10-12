import { toast } from 'svelte-sonner';
import { SvelteSet } from 'svelte/reactivity';
import {
	apiErrorMessage,
	type CreateRequestRequest,
	type DiscoverDetailResponse,
	type DiscoverItemResponse,
	type DiscoverSeasonResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import { MediaKind } from '$lib/catalog/title-page.svelte';
import { SDK_REQUESTS_API, type RequestsApi } from './requests-api';
import {
	OPEN_STATUSES,
	RequestProblem,
	RequestScope,
	RequestStatus,
	conflictOf,
	mediaKey
} from './requests';

/** What a TMDB card in the top-bar search offers. */
export enum CardState {
	IN_LIBRARY = 'IN_LIBRARY',
	REQUESTED = 'REQUESTED',
	REQUESTABLE = 'REQUESTABLE'
}

const SPECIALS_SEASON = 0;

/** The series scope picker: the whole series, or the seasons ticked. */
export class ScopePicker {
	status = $state(LoadStatus.LOADING);
	error = $state('');
	seasons = $state<DiscoverSeasonResponse[]>([]);
	wholeSeries = $state(true);
	readonly chosen = new SvelteSet<number>();

	readonly item: DiscoverItemResponse;
	readonly #api: RequestsApi;

	constructor(item: DiscoverItemResponse, api: RequestsApi) {
		this.item = item;
		this.#api = api;
	}

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		try {
			const detail: DiscoverDetailResponse = await this.#api.media(
				this.item.kind,
				this.item.tmdbId
			);
			this.seasons = detail.seasons;
			this.status = LoadStatus.READY;
		} catch (error) {
			this.error = apiErrorMessage(error, 'Could not load the seasons.');
			this.status = LoadStatus.ERROR;
		}
	}

	seasonLabel(season: DiscoverSeasonResponse): string {
		return season.number === SPECIALS_SEASON ? 'Specials' : `Season ${season.number}`;
	}

	toggle(number: number, on: boolean): void {
		if (on) this.chosen.add(number);
		else this.chosen.delete(number);
		this.wholeSeries = false;
	}

	get canSubmit(): boolean {
		return this.wholeSeries || this.chosen.size > 0;
	}

	body(): CreateRequestRequest {
		const base = { kind: this.item.kind, tmdbId: this.item.tmdbId };
		return this.wholeSeries
			? { ...base, scope: RequestScope.ALL_SEASONS }
			: { ...base, scope: RequestScope.SEASONS, seasons: [...this.chosen].sort((a, b) => a - b) };
	}
}

/**
 * Add (admins) and Request (requesters) on the top-bar search's TMDB cards. Movies are requested at once;
 * series open the scope picker first.
 */
export class SearchRequests {
	readonly requested = new SvelteSet<string>();
	/** Cards with a request in flight. */
	readonly sending = new SvelteSet<string>();
	picker = $state<ScopePicker>();
	submitting = $state(false);

	readonly #api: RequestsApi;

	constructor(api: RequestsApi = SDK_REQUESTS_API) {
		this.#api = api;
	}

	/** Remembers the viewer's open requests, so their cards read Requested. */
	async load(): Promise<void> {
		try {
			const open = await this.#api.list(OPEN_STATUSES);
			this.requested.clear();
			for (const request of open.items) this.requested.add(mediaKey(request.kind, request.tmdbId));
		} catch {
			// Cards stay requestable; a duplicate request answers 409 and is marked then.
		}
	}

	stateOf(item: DiscoverItemResponse): CardState {
		if (item.inLibrary) return CardState.IN_LIBRARY;
		return this.requested.has(mediaKey(item.kind, item.tmdbId))
			? CardState.REQUESTED
			: CardState.REQUESTABLE;
	}

	isSending(item: DiscoverItemResponse): boolean {
		return this.sending.has(mediaKey(item.kind, item.tmdbId));
	}

	async request(item: DiscoverItemResponse): Promise<void> {
		if (item.kind === MediaKind.SERIES) {
			this.picker = new ScopePicker(item, this.#api);
			await this.picker.load();
			return;
		}
		await this.#send(item, { kind: item.kind, tmdbId: item.tmdbId, scope: RequestScope.MOVIE });
	}

	async submitPicker(): Promise<void> {
		const picker = this.picker;
		if (!picker?.canSubmit) return;
		this.submitting = true;
		try {
			if (await this.#send(picker.item, picker.body())) this.picker = undefined;
		} finally {
			this.submitting = false;
		}
	}

	closePicker(): void {
		this.picker = undefined;
	}

	async #send(item: DiscoverItemResponse, body: CreateRequestRequest): Promise<boolean> {
		const key = mediaKey(item.kind, item.tmdbId);
		this.sending.add(key);
		try {
			const created = await this.#api.create(body);
			this.requested.add(key);
			toast.success(
				created.status === RequestStatus.PENDING
					? `Requested ${item.name}. An admin will review it.`
					: `Adding ${item.name}`
			);
			return true;
		} catch (error) {
			switch (conflictOf(error)) {
				case RequestProblem.ALREADY_REQUESTED:
					this.requested.add(key);
					toast.info(`${item.name} is already requested`);
					return true;
				case RequestProblem.ALREADY_MONITORED:
					toast.info(`${item.name} is already in the library`);
					return true;
				default:
					toast.error(apiErrorMessage(error, `Could not request ${item.name}.`));
					return false;
			}
		} finally {
			this.sending.delete(key);
		}
	}
}
