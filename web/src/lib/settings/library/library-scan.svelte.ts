import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	getLibraryScan,
	setLibraryScanOptions,
	startLibraryScan,
	type LibraryScanResponse,
	type ScanOptionsRequest
} from '$lib/sdk';
import type { LiveConnection } from '$lib/live/live-connection';
import { LoadStatus } from '$lib/load-status';
import { LibraryEvent } from '$lib/library/library';

export interface LibraryScanApi {
	status: () => Promise<LibraryScanResponse>;
	start: () => Promise<LibraryScanResponse>;
	setOptions: (request: ScanOptionsRequest) => Promise<LibraryScanResponse>;
}

export interface Notifier {
	success: (message: string, options?: { description?: string }) => void;
	error: (message: string, options?: { description?: string }) => void;
}

export type ScanLive = Pick<LiveConnection, 'on' | 'onReset'>;

export const SCAN_FOLDER_MATCHED_EVENT = 'catalog.ScanFolderMatched';

/** How often the scan refreshes while it runs or its folders are still being matched. */
export const SCAN_POLL_MS = 2_000;
/** How long after a scan's walk Catalog may still be matching its folders against TMDB. */
export const SCAN_SETTLE_MS = 30_000;
/** A scan sends an event per folder, so a burst of them causes one refresh. */
export const SCAN_EVENT_SETTLE_MS = 500;

const sdkApi: LibraryScanApi = {
	status: () => getLibraryScan(),
	start: () => startLibraryScan(),
	setOptions: (request) => setLibraryScanOptions(request)
};

/** State behind Settings → Library's scan card: the last scan, its counts and the rename option. */
export class LibraryScanManager {
	scan = $state<LibraryScanResponse>();
	status = $state(LoadStatus.LOADING);
	error = $state('');
	starting = $state(false);

	readonly #api: LibraryScanApi;
	readonly #notify: Notifier;
	readonly #now: () => number;
	#changed: () => void = () => {};
	#timer: ReturnType<typeof setTimeout> | undefined;
	#watching = false;

	constructor(
		api: LibraryScanApi = sdkApi,
		notify: Notifier = toast,
		now: () => number = Date.now
	) {
		this.#api = api;
		this.#notify = notify;
		this.#now = now;
	}

	get running(): boolean {
		return this.scan?.running ?? false;
	}

	/** True while a scan runs, and for a while after it, as Catalog matches the folders it found. */
	get settling(): boolean {
		if (this.running) return true;
		const finished = this.scan?.finishedAt;
		return !!finished && this.#now() - Date.parse(finished) < SCAN_SETTLE_MS;
	}

	async load(): Promise<void> {
		try {
			this.scan = await this.#api.status();
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			if (this.status !== LoadStatus.READY) this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load the library scan.');
		}
	}

	/** Reloads the scan, tells the review queue to reload too, and keeps polling while the scan settles. */
	async refresh(): Promise<void> {
		await this.load();
		this.#changed();
		this.#schedule();
	}

	async start(): Promise<void> {
		this.starting = true;
		try {
			this.scan = await this.#api.start();
			this.#notify.success('Scanning the library', {
				description: 'Matched folders appear in the library; uncertain ones wait below.'
			});
			this.#schedule();
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not start the scan.'));
		} finally {
			this.starting = false;
		}
	}

	async setRenameMatched(renameMatched: boolean): Promise<void> {
		if (!this.scan) return;
		const before = this.scan.renameMatched;
		this.scan.renameMatched = renameMatched;
		try {
			this.scan = await this.#api.setOptions({ renameMatched });
		} catch (error) {
			this.scan.renameMatched = before;
			this.#notify.error(apiErrorMessage(error, 'Could not save the option.'));
		}
	}

	/** Refreshes on scan events and polls while the scan settles; changed runs after each refresh. Returns a cleanup. */
	watch(live: ScanLive, changed: () => void): () => void {
		this.#changed = changed;
		this.#watching = true;
		const soon = () => this.#refreshIn(SCAN_EVENT_SETTLE_MS);
		const stops = [
			live.on(SCAN_FOLDER_MATCHED_EVENT, soon),
			live.on(LibraryEvent.FOLDER_FOUND, soon),
			live.onReset(() => void this.refresh())
		];
		this.#schedule();
		return () => {
			this.#watching = false;
			clearTimeout(this.#timer);
			for (const stop of stops) stop();
		};
	}

	#schedule(): void {
		if (this.settling) this.#refreshIn(SCAN_POLL_MS);
		else clearTimeout(this.#timer);
	}

	#refreshIn(ms: number): void {
		clearTimeout(this.#timer);
		if (this.#watching) this.#timer = setTimeout(() => void this.refresh(), ms);
	}
}
