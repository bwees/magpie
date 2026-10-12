import { SvelteSet } from 'svelte/reactivity';
import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	importManually,
	listImports,
	removeImport,
	retryImport,
	type FileImported,
	type ImportResponse,
	type ManualImportRequest
} from '$lib/sdk';
import type { LiveConnection } from '$lib/live/live-connection';
import { LoadStatus } from '$lib/load-status';
import { importVideos, LibraryEvent, MediaKind, STOPPED_IMPORTS } from './library';

export interface ImportsApi {
	list: () => Promise<ImportResponse[]>;
	retry: (download: string) => Promise<unknown>;
	remove: (download: string) => Promise<unknown>;
	map: (download: string, request: ManualImportRequest) => Promise<unknown>;
}

export interface Notifier {
	success: (message: string, options?: { description?: string }) => void;
	error: (message: string, options?: { description?: string }) => void;
}

const STATUS_SEPARATOR = ',';

const sdkApi: ImportsApi = {
	list: () => listImports(STOPPED_IMPORTS.join(STATUS_SEPARATOR)),
	retry: (download) => retryImport(download),
	remove: (download) => removeImport(download),
	map: (download, request) => importManually(download, request)
};

export type ImportsLive = Pick<LiveConnection, 'on' | 'onReset'>;

/** One video's row in the mapping picker. */
export interface VideoChoice {
	path: string;
	included: boolean;
	episodes: string[];
}

/** State and actions behind Activity → Imports: failed and manual imports, Retry, Remove and Map files. */
export class ImportsManager {
	imports = $state<ImportResponse[]>([]);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	readonly busy = new SvelteSet<string>();

	/** The import whose mapping picker is open. */
	mapping = $state<ImportResponse | undefined>();
	choices = $state<VideoChoice[]>([]);
	mappingError = $state<string | undefined>();
	submitting = $state(false);

	readonly #api: ImportsApi;
	readonly #notify: Notifier;

	constructor(api: ImportsApi = sdkApi, notify: Notifier = toast) {
		this.#api = api;
		this.#notify = notify;
	}

	get count(): number {
		return this.imports.length;
	}

	get mappingMovie(): boolean {
		return this.mapping?.kind === MediaKind.MOVIE;
	}

	async load(): Promise<void> {
		try {
			this.imports = await this.#api.list();
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			if (this.status !== LoadStatus.READY) this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load the imports.');
		}
	}

	/** Returns a cleanup. */
	watch(live: ImportsLive): () => void {
		const reload = () => void this.load();
		const stops = [
			live.on(LibraryEvent.IMPORT_FAILED, reload),
			live.on(LibraryEvent.FILE_IMPORTED, (m) => {
				const download = (m.data as FileImported).download;
				if (download && this.imports.some((imp) => imp.download === download)) reload();
			}),
			live.onReset(reload)
		];
		return () => stops.forEach((stop) => stop());
	}

	async retry(imp: ImportResponse): Promise<void> {
		await this.#act(imp, () => this.#api.retry(imp.download), 'Import queued again');
	}

	async remove(imp: ImportResponse): Promise<void> {
		await this.#act(imp, () => this.#api.remove(imp.download), 'Import removed');
	}

	openMapping(imp: ImportResponse): void {
		this.mapping = imp;
		this.mappingError = undefined;
		const videos = importVideos(imp);
		const movie = imp.kind === MediaKind.MOVIE;
		const firstPlayable = videos.find((v) => !v.sample) ?? videos[0];
		this.choices = videos.map((v) => ({
			path: v.path,
			included: movie ? v === firstPlayable : v.episodes.length > 0 || !v.sample,
			episodes: v.episodes.map((e) => e.episode)
		}));
	}

	closeMapping(): void {
		this.mapping = undefined;
	}

	/** A movie takes one video, so choosing one clears the others; a series takes any. */
	toggleVideo(path: string, on: boolean): void {
		for (const choice of this.choices) {
			if (choice.path === path) {
				choice.included = on || this.mappingMovie;
			} else if (this.mappingMovie) {
				choice.included = false;
			}
		}
	}

	/** Why the mapping can't be sent yet, or undefined. */
	get mappingProblem(): string | undefined {
		const included = this.choices.filter((c) => c.included);
		if (included.length === 0) {
			return 'Pick at least one video to import';
		}
		if (!this.mappingMovie) {
			const missing = included.find((c) => c.episodes.length === 0);
			if (missing) {
				return `Pick the episodes of ${missing.path}`;
			}
			const episodes = included.flatMap((c) => c.episodes);
			if (episodes.some((e, i) => episodes.indexOf(e) !== i)) {
				return 'Two videos can’t be the same episode';
			}
		}
		return undefined;
	}

	async submitMapping(): Promise<void> {
		const imp = this.mapping;
		this.mappingError = this.mappingProblem;
		if (!imp || this.mappingError) {
			return;
		}
		const request: ManualImportRequest = {
			files: this.choices
				.filter((c) => c.included)
				.map((c) => ({ path: c.path, episodes: this.mappingMovie ? [] : c.episodes }))
		};
		this.submitting = true;
		try {
			await this.#api.map(imp.download, request);
			this.mapping = undefined;
			this.#notify.success('Import queued with your mapping', { description: imp.releaseName });
			await this.load();
		} catch (error) {
			this.mappingError = apiErrorMessage(error, 'Could not import with this mapping.');
		} finally {
			this.submitting = false;
		}
	}

	async #act(imp: ImportResponse, action: () => Promise<unknown>, done: string): Promise<void> {
		this.busy.add(imp.download);
		try {
			await action();
			this.imports = this.imports.filter((i) => i.download !== imp.download);
			this.#notify.success(done, { description: imp.releaseName });
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Something went wrong. Please try again.'), {
				description: imp.releaseName
			});
		} finally {
			this.busy.delete(imp.download);
		}
	}
}
