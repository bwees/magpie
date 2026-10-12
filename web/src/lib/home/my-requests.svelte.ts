import { SvelteMap } from 'svelte/reactivity';
import type {
	DiscoverDetailResponse,
	PipelineRowResponse,
	ProjectionUpdated,
	RequestResponse
} from '$lib/sdk';
import { DataBlock } from '$lib/data-block.svelte';
import { LiveEventType, Projection, type LiveEvents } from '$lib/activity/activity-source';
import { SDK_FEEDS, type ActivityFeeds } from '$lib/activity/feeds';
import { unique } from '$lib/catalog/title-directory.svelte';
import { STAGE_LABELS, Stage } from '$lib/activity/pipeline';
import { SDK_REQUESTS_API, type RequestsApi } from '$lib/requests/requests-api';
import {
	ITEM_AVAILABLE_EVENT,
	ITEM_AVAILABLE_SETTLE_MS,
	REQUEST_EVENTS,
	RequestStatus,
	mediaKey,
	statusLabel
} from '$lib/requests/requests';

/** Statuses whose request is on its way, so the pipeline stage says more than the status. */
const IN_FLIGHT: string[] = [RequestStatus.APPROVED, RequestStatus.PROCESSING];
const FAILED: string[] = [RequestStatus.FAILED, RequestStatus.DECLINED];

export interface RequestBadge {
	label: string;
	failed: boolean;
	/** The pipeline's live detail, e.g. "Waiting for Jellyfin". */
	detail: string;
}

/** A requester's Home: their own requests, each with its live pipeline stage. */
export class MyRequests {
	readonly block: DataBlock<RequestResponse[]>;
	/** Name, year and poster per movie or series, keyed by mediaKey. */
	readonly media = new SvelteMap<string, DiscoverDetailResponse>();
	/** Each requested title's summary row. */
	readonly stages = new SvelteMap<string, PipelineRowResponse>();

	readonly #userId: string;
	readonly #requests: RequestsApi;
	readonly #feeds: ActivityFeeds;

	constructor(
		userId: string,
		requests: RequestsApi = SDK_REQUESTS_API,
		feeds: ActivityFeeds = SDK_FEEDS
	) {
		this.#userId = userId;
		this.#requests = requests;
		this.#feeds = feeds;
		this.block = new DataBlock({
			fetch: () => this.#fetch(),
			initial: [],
			isEmpty: (items) => items.length === 0,
			failure: 'Could not load your requests.'
		});
	}

	get items(): RequestResponse[] {
		return this.block.data;
	}

	load(): Promise<void> {
		return this.block.load();
	}

	async #fetch(): Promise<RequestResponse[]> {
		const all = await this.#requests.list([]);
		const mine = all.items.filter((request) => request.requester.id === this.#userId);
		await Promise.all([this.#loadMedia(mine), this.#loadStages(this.#titlesOf(mine))]);
		return mine;
	}

	#titlesOf(requests: RequestResponse[]): string[] {
		return unique(requests.flatMap((request) => (request.titleId ? [request.titleId] : [])));
	}

	async #loadMedia(requests: RequestResponse[]): Promise<void> {
		const missing = requests.filter((r) => !this.media.has(mediaKey(r.kind, r.tmdbId)));
		await Promise.all(
			missing.map(async (request) => {
				try {
					this.media.set(
						mediaKey(request.kind, request.tmdbId),
						await this.#requests.media(request.kind, request.tmdbId)
					);
				} catch {
					// The row falls back to the TMDB ID.
				}
			})
		);
	}

	async #loadStages(titles: string[]): Promise<void> {
		try {
			for (const row of await this.#feeds.stages(titles)) this.stages.set(row.titleId, row);
		} catch {
			// Rows show their request status until the next update.
		}
	}

	mediaOf(request: RequestResponse): DiscoverDetailResponse | undefined {
		return this.media.get(mediaKey(request.kind, request.tmdbId));
	}

	nameOf(request: RequestResponse): string {
		return this.mediaOf(request)?.name ?? `TMDB ${request.tmdbId}`;
	}

	/** The stage while the request is on its way; the request status otherwise. */
	badge(request: RequestResponse): RequestBadge {
		const row = request.titleId ? this.stages.get(request.titleId) : undefined;
		if (row && IN_FLIGHT.includes(request.status)) {
			return {
				label: STAGE_LABELS[row.stage as Stage] ?? row.stage,
				failed: row.failed,
				detail: row.detail
			};
		}
		return { label: statusLabel(request), failed: FAILED.includes(request.status), detail: '' };
	}

	/** Follows request and pipeline events until the returned function is called. */
	watch(live: LiveEvents): () => void {
		let settle: ReturnType<typeof setTimeout> | undefined;
		const stops = REQUEST_EVENTS.map((type) => live.on(type, () => void this.block.refresh()));
		stops.push(
			live.on(ITEM_AVAILABLE_EVENT, () => {
				clearTimeout(settle);
				settle = setTimeout(() => void this.block.refresh(), ITEM_AVAILABLE_SETTLE_MS);
			}),
			() => clearTimeout(settle),
			live.on(LiveEventType.PROJECTION_UPDATED, (message) => {
				const update = message.data as ProjectionUpdated;
				if (
					update.projection === Projection.PIPELINE &&
					update.title &&
					this.stages.has(update.title)
				) {
					void this.#loadStages([update.title]);
				}
			}),
			live.onReset(() => void this.block.refresh())
		);
		return () => stops.forEach((stop) => stop());
	}
}
