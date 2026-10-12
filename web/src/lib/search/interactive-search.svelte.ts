import { toast } from 'svelte-sonner';
import { SvelteSet } from 'svelte/reactivity';
import {
	apiErrorMessage,
	cancelSearch,
	getSearch,
	grabRelease,
	startSearch,
	type GrabRequest,
	type ReleaseDecision,
	type SearchCompleted,
	type SearchIndexerResponse,
	type SearchProfileResponse,
	type SearchResponse,
	type SearchResultsReceived
} from '$lib/sdk';
import type { LiveConnection } from '$lib/live/live-connection';
import { searchTopic, type LiveMessage } from '$lib/live/protocol';
import { bestFirst, mergeRanked, rankedOrder, unseen } from './release-order';

export enum IndexerStatus {
	PENDING = 'PENDING',
	DONE = 'DONE',
	FAILED = 'FAILED',
	SKIPPED = 'SKIPPED'
}

export enum SearchEventType {
	RESULTS = 'acquisition.SearchResultsReceived',
	COMPLETED = 'acquisition.SearchCompleted'
}

export enum SearchPhase {
	IDLE = 'IDLE',
	STARTING = 'STARTING',
	RUNNING = 'RUNNING',
	FINISHED = 'FINISHED',
	FAILED = 'FAILED'
}

/** A late answer turns FAILED into DONE; a replayed event never moves an indexer back. */
const STATUS_WEIGHT: Record<string, number> = {
	[IndexerStatus.PENDING]: 0,
	[IndexerStatus.FAILED]: 1,
	[IndexerStatus.SKIPPED]: 1,
	[IndexerStatus.DONE]: 2
};

export interface SearchApi {
	start(titleId: string): Promise<SearchResponse>;
	get(searchId: string): Promise<SearchResponse>;
	cancel(searchId: string): Promise<unknown>;
	grab(searchId: string, request: GrabRequest): Promise<unknown>;
}

export const ACQUISITION_SEARCH_API: SearchApi = {
	start: (title) => startSearch({ title, interactive: true }),
	get: (id) => getSearch(id),
	cancel: (id) => cancelSearch(id),
	grab: (id, request) => grabRelease(id, request)
};

/** The parts of the live socket a search uses. */
export type SearchLive = Pick<LiveConnection, 'on' | 'onReset' | 'subscribe' | 'unsubscribe'>;

export interface QualityBucket {
	name: string;
	meetsCutoff: boolean;
	/** Arrival order; rows never move once shown. */
	releases: ReleaseDecision[];
	/** The highest ranked release in the bucket. */
	best?: string;
}

export function isAccepted(release: ReleaseDecision): boolean {
	return (release.rejections ?? []).length === 0;
}

/** One interactive search: starting it, streaming its results from the live socket, and grabbing. */
export class InteractiveSearch {
	phase = $state(SearchPhase.IDLE);
	error = $state('');
	searchId = $state<string | undefined>();
	profile = $state<SearchProfileResponse | undefined>();
	indexers = $state<SearchIndexerResponse[]>([]);
	/** Every release in arrival order. */
	releases = $state<ReleaseDecision[]>([]);
	/** Accepted release IDs, best first. */
	order = $state<string[]>([]);
	completed = $state(false);
	readonly grabbing = new SvelteSet<string>();

	readonly answered = $derived(
		this.indexers.filter((i) => i.status !== IndexerStatus.PENDING).length
	);
	readonly failures = $derived(
		this.indexers.filter((i) => i.status === IndexerStatus.FAILED).length
	);
	readonly accepted = $derived(this.releases.filter(isAccepted));
	readonly rejected = $derived(this.releases.filter((r) => !isAccepted(r)));
	readonly allFailed = $derived(
		this.completed &&
			this.indexers.length > 0 &&
			this.indexers.every((i) => i.status === IndexerStatus.FAILED)
	);
	readonly noIndexers = $derived(this.completed && this.indexers.length === 0);
	readonly buckets = $derived.by(() => this.#buckets());

	readonly #titleId: string;
	readonly #live: SearchLive;
	readonly #api: SearchApi;
	#unwatch: (() => void) | undefined;

	constructor(titleId: string, live: SearchLive, api: SearchApi = ACQUISITION_SEARCH_API) {
		this.#titleId = titleId;
		this.#live = live;
		this.#api = api;
	}

	get running(): boolean {
		return this.phase === SearchPhase.STARTING || this.phase === SearchPhase.RUNNING;
	}

	isPenalized(format: string): boolean {
		return this.profile?.penalizedFormats.includes(format) ?? false;
	}

	/** Starts a fresh search, cancelling the one before it. */
	async start(): Promise<void> {
		this.stop();
		this.#reset();
		this.phase = SearchPhase.STARTING;
		this.#watch();
		try {
			const started = await this.#api.start(this.#titleId);
			this.#apply(started);
			this.#live.subscribe(searchTopic(started.searchId));
			// Answers sent before the subscription reached the server are in the cached state.
			this.#apply(await this.#api.get(started.searchId));
		} catch (error) {
			this.phase = SearchPhase.FAILED;
			this.error = apiErrorMessage(error, 'Could not start the search.');
		}
	}

	/** Leaves the search: cancels what is still running and stops listening. */
	stop(): void {
		this.#unwatch?.();
		this.#unwatch = undefined;
		const id = this.searchId;
		if (!id) {
			return;
		}
		this.searchId = undefined;
		this.phase = SearchPhase.IDLE;
		this.#live.unsubscribe(searchTopic(id));
		if (!this.completed) {
			this.#api.cancel(id).catch(() => undefined);
		}
	}

	async grab(release: ReleaseDecision): Promise<void> {
		const id = this.searchId;
		if (!id) {
			return;
		}
		this.grabbing.add(release.release);
		try {
			await this.#api.grab(id, { releaseId: release.release, override: !isAccepted(release) });
			toast.success('Grab sent', { description: release.name });
		} catch (error) {
			toast.error(apiErrorMessage(error, 'Could not grab the release.'), {
				description: release.name
			});
		} finally {
			this.grabbing.delete(release.release);
		}
	}

	#reset(): void {
		this.searchId = undefined;
		this.error = '';
		this.indexers = [];
		this.releases = [];
		this.order = [];
		this.completed = false;
		this.grabbing.clear();
	}

	#watch(): void {
		const onResults = (message: LiveMessage) =>
			this.#onResults(message.data as SearchResultsReceived);
		const onCompleted = (message: LiveMessage) =>
			this.#onCompleted(message.data as SearchCompleted);
		const stops = [
			this.#live.on(SearchEventType.RESULTS, onResults),
			this.#live.on(SearchEventType.COMPLETED, onCompleted),
			this.#live.onReset(() => void this.#refresh())
		];
		this.#unwatch = () => stops.forEach((stop) => stop());
	}

	async #refresh(): Promise<void> {
		const id = this.searchId;
		if (!id) {
			return;
		}
		try {
			this.#apply(await this.#api.get(id));
		} catch (error) {
			this.phase = SearchPhase.FAILED;
			this.error = apiErrorMessage(error, 'The search results are no longer available.');
		}
	}

	#apply(snapshot: SearchResponse): void {
		this.searchId = snapshot.searchId;
		this.profile = snapshot.profile;
		for (const indexer of snapshot.indexers) {
			this.#setStatus(indexer);
		}
		const newer = unseen(snapshot.releases, this.releases);
		this.#add(snapshot.releases);
		this.order = mergeRanked(rankedOrder(snapshot.releases), newer);
		this.#complete(snapshot.completed);
	}

	#onResults(event: SearchResultsReceived): void {
		if (event.search !== this.searchId) {
			return;
		}
		this.#setStatus({ name: event.indexer, status: event.status, message: event.message });
		const releases = event.releases ?? [];
		this.#add(releases);
		this.order = mergeRanked(this.order, releases);
	}

	#onCompleted(event: SearchCompleted): void {
		if (event.search === this.searchId) {
			this.#complete(true);
		}
	}

	#complete(completed: boolean): void {
		this.completed = this.completed || completed;
		this.phase = this.completed ? SearchPhase.FINISHED : SearchPhase.RUNNING;
	}

	#setStatus(update: SearchIndexerResponse): void {
		const current = this.indexers.find((i) => i.name === update.name);
		if (!current) {
			this.indexers.push({ ...update });
			return;
		}
		if (STATUS_WEIGHT[update.status] >= STATUS_WEIGHT[current.status]) {
			current.status = update.status;
			current.message = update.message;
		}
	}

	#add(releases: ReleaseDecision[]): void {
		this.releases.push(...unseen(this.releases, releases));
	}

	#buckets(): QualityBucket[] {
		return (this.profile?.buckets ?? []).map((bucket) => {
			const releases = this.accepted.filter((r) => bucket.qualityIds.includes(r.quality.id));
			const best = bestFirst(this.order, releases)[0]?.release;
			return { name: bucket.name, meetsCutoff: bucket.meetsCutoff, releases, best };
		});
	}
}
