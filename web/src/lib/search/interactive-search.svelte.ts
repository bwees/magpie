import { toast } from 'svelte-sonner';
import { SvelteMap } from 'svelte/reactivity';
import {
	apiErrorMessage,
	cancelSearch,
	getSearch,
	grabRelease,
	startSearch,
	type DownloadProgressed,
	type GrabRequest,
	type GrabResponse,
	type ReleaseDecision,
	type ReleaseGrabbed,
	type ReleaseRejected,
	type SearchCompleted,
	type SearchIndexerResponse,
	type SearchProfileResponse,
	type SearchResponse,
	type SearchResultsReceived
} from '$lib/sdk';
import type { LiveConnection } from '$lib/live/live-connection';
import { searchTopic, type LiveMessage } from '$lib/live/protocol';
import { bestFirst, mergeRanked, rankedOrder, unseen } from './release-order';
import type { SearchTarget } from './search-target';

export enum IndexerStatus {
	PENDING = 'PENDING',
	DONE = 'DONE',
	FAILED = 'FAILED',
	SKIPPED = 'SKIPPED'
}

export enum SearchEventType {
	RESULTS = 'acquisition.SearchResultsReceived',
	COMPLETED = 'acquisition.SearchCompleted',
	GRABBED = 'acquisition.ReleaseGrabbed',
	REJECTED = 'acquisition.ReleaseRejected',
	PROGRESSED = 'acquisition.DownloadProgressed'
}

/** Where a grabbed release stands. Mirrors the Go `GrabStatus` values. */
export enum GrabState {
	/** The grab request is in flight, or a magnet waits for its file list. */
	CHECKING = 'CHECKING',
	DOWNLOADING = 'DOWNLOADING',
	REJECTED = 'REJECTED'
}

/** Mirrors the Go `RejectionReason` values a grabbed release can be dropped for. */
export enum DropReason {
	BLOCKED_FILE = 'BLOCKED_FILE',
	METADATA_TIMEOUT = 'METADATA_TIMEOUT',
	STALLED = 'STALLED'
}

const AWAITING_METADATA = 'AWAITING_METADATA';

function dropMessage(reason: string, blockedFile?: string): string {
	switch (reason) {
		case DropReason.BLOCKED_FILE:
			return `Rejected: ${blockedFile ?? 'a file'} is a blocked file type`;
		case DropReason.METADATA_TIMEOUT:
			return 'Rejected: the file list never arrived';
		case DropReason.STALLED:
			return 'Rejected: the download stalled';
	}
	return 'Rejected: the download failed';
}

function nextMessage(next?: string): string {
	return next ? 'Grabbing the next best release.' : 'Nothing else cached; searching again.';
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
	start(target: SearchTarget): Promise<SearchResponse>;
	get(searchId: string): Promise<SearchResponse>;
	cancel(searchId: string): Promise<unknown>;
	grab(searchId: string, request: GrabRequest): Promise<GrabResponse>;
}

export const ACQUISITION_SEARCH_API: SearchApi = {
	start: (target) =>
		startSearch({ title: target.titleId, episodes: target.episodeIds, interactive: true }),
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
	/** Grab state by release ID, for releases grabbed by the user or by the fallback. */
	readonly grabs = new SvelteMap<string, GrabState>();
	/** The release behind each download this search grabbed. */
	readonly #grabbedAs = new SvelteMap<string, string>();

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

	readonly #live: SearchLive;
	readonly #api: SearchApi;
	#titleId: string | undefined;
	#unwatch: (() => void) | undefined;

	constructor(live: SearchLive, api: SearchApi = ACQUISITION_SEARCH_API) {
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
	async start(target: SearchTarget): Promise<void> {
		this.stop();
		this.#reset();
		this.#titleId = target.titleId;
		this.phase = SearchPhase.STARTING;
		this.#watch();
		try {
			const started = await this.#api.start(target);
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
		this.grabs.set(release.release, GrabState.CHECKING);
		try {
			const result = await this.#api.grab(id, {
				releaseId: release.release,
				override: !isAccepted(release)
			});
			this.#applyGrab(release, result);
		} catch (error) {
			this.grabs.delete(release.release);
			toast.error(apiErrorMessage(error, 'Could not grab the release.'), {
				description: release.name
			});
		}
	}

	grabState(release: ReleaseDecision): GrabState | undefined {
		return this.grabs.get(release.release);
	}

	#applyGrab(release: ReleaseDecision, result: GrabResponse): void {
		if (result.download) {
			this.#grabbedAs.set(result.download, release.release);
		}
		if (result.status === GrabState.REJECTED) {
			this.#reject(release, DropReason.BLOCKED_FILE, result.blockedFile, result.next);
			return;
		}
		if (this.grabs.get(release.release) === GrabState.CHECKING) {
			this.grabs.set(release.release, result.status as GrabState);
		}
		if (result.status === GrabState.DOWNLOADING) {
			toast.success('Downloading', { description: release.name });
		}
	}

	/** Marks a release rejected once, however its rejection arrives first: the grab answer or the event. */
	#reject(release: ReleaseDecision, reason: string, blockedFile?: string, next?: string): void {
		if (this.grabs.get(release.release) === GrabState.REJECTED) {
			return;
		}
		this.grabs.set(release.release, GrabState.REJECTED);
		toast.error(dropMessage(reason, blockedFile), {
			description: `${release.name}. ${nextMessage(next)}`
		});
	}

	#releaseNamed(name: string, indexerName: string): ReleaseDecision | undefined {
		return this.releases.find((r) => r.name === name && r.indexer === indexerName);
	}

	/** A grab this dialog didn't ask for is the fallback after a rejection. */
	#onGrabbed(event: ReleaseGrabbed): void {
		const release = this.#releaseNamed(event.release, event.indexer);
		if (event.title !== this.#titleId || !release) {
			return;
		}
		this.#grabbedAs.set(event.download, release.release);
		if (!this.grabs.has(release.release)) {
			this.grabs.set(release.release, GrabState.CHECKING);
			toast.success('Grabbed the next best release', { description: release.name });
		}
	}

	#onRejected(event: ReleaseRejected): void {
		const release = this.#releaseNamed(event.release, event.indexer);
		if (event.title !== this.#titleId || !release || !this.grabs.has(release.release)) {
			return;
		}
		this.#reject(release, event.reason, event.blockedFile ?? undefined, event.next ?? undefined);
	}

	#onProgressed(event: DownloadProgressed): void {
		const releaseId = this.#grabbedAs.get(event.download);
		if (
			releaseId &&
			event.state !== AWAITING_METADATA &&
			this.grabs.get(releaseId) === GrabState.CHECKING
		) {
			this.grabs.set(releaseId, GrabState.DOWNLOADING);
		}
	}

	#reset(): void {
		this.searchId = undefined;
		this.error = '';
		this.indexers = [];
		this.releases = [];
		this.order = [];
		this.completed = false;
		this.grabs.clear();
		this.#grabbedAs.clear();
	}

	#watch(): void {
		const onResults = (message: LiveMessage) =>
			this.#onResults(message.data as SearchResultsReceived);
		const onCompleted = (message: LiveMessage) =>
			this.#onCompleted(message.data as SearchCompleted);
		const stops = [
			this.#live.on(SearchEventType.RESULTS, onResults),
			this.#live.on(SearchEventType.COMPLETED, onCompleted),
			this.#live.on(SearchEventType.GRABBED, (m) => this.#onGrabbed(m.data as ReleaseGrabbed)),
			this.#live.on(SearchEventType.REJECTED, (m) => this.#onRejected(m.data as ReleaseRejected)),
			this.#live.on(SearchEventType.PROGRESSED, (m) =>
				this.#onProgressed(m.data as DownloadProgressed)
			),
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
