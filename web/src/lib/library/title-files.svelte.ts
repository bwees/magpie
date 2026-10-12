import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	deleteMediaFile,
	listCutoffStatus,
	listMediaFiles,
	type CutoffMet,
	type CutoffStatusResponse,
	type EpisodeGroupChanged,
	type FileDeleted,
	type FileImported,
	type FileTranscoded,
	type ItemAvailable,
	type MediaFileResponse
} from '$lib/sdk';
import type { LiveConnection } from '$lib/live/live-connection';
import { LoadStatus } from '$lib/load-status';
import type { Notifier } from './imports-manager.svelte';
import { LibraryEvent } from './library';

export interface FilesApi {
	list: (title: string) => Promise<MediaFileResponse[]>;
	cutoff: (title: string) => Promise<CutoffStatusResponse[]>;
	remove: (file: string) => Promise<unknown>;
}

const sdkApi: FilesApi = {
	list: (title) => listMediaFiles({ title }),
	cutoff: (title) => listCutoffStatus(title),
	remove: (file) => deleteMediaFile(file)
};

export type FilesLive = Pick<LiveConnection, 'on' | 'onReset'>;

export const EPISODE_GROUP_CHANGED_EVENT = 'catalog.EpisodeGroupChanged';
export const CUTOFF_MET_EVENT = 'acquisition.CutoffMet';
/** Library renames the files in a job after the renumbering commits, so the tab reloads a moment later. */
export const RENAME_SETTLE_MS = 1_500;

/** Where a file stands against its profile's cutoff. */
export enum CutoffState {
	/** Good enough: it transcodes and stops upgrading. */
	MET = 'MET',
	/** Below the cutoff; Magpie keeps looking for a better release. */
	UPGRADING = 'UPGRADING',
	/** Below the cutoff, but it won't be upgraded: transcoded, or the profile doesn't upgrade. */
	KEPT = 'KEPT',
	/** Acquisition hasn't scored the file yet. */
	UNKNOWN = 'UNKNOWN'
}

export function cutoffState(status: CutoffStatusResponse | undefined): CutoffState {
	if (!status) return CutoffState.UNKNOWN;
	if (status.cutoffMet) return CutoffState.MET;
	return status.upgradesAllowed ? CutoffState.UPGRADING : CutoffState.KEPT;
}

/** The title page's Files tab: the title's media files with their cutoff state, and deleting one. */
export class TitleFiles {
	files = $state<MediaFileResponse[]>([]);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	readonly cutoff = new SvelteMap<string, CutoffStatusResponse>();
	readonly deleting = new SvelteSet<string>();
	/** The file whose delete dialog is open. */
	confirmingDelete = $state<MediaFileResponse | undefined>();

	/** Files Jellyfin still hasn't confirmed a day after their import. */
	get overdue(): MediaFileResponse[] {
		return this.files.filter((file) => file.jellyfinOverdue);
	}

	readonly #title: string;
	readonly #api: FilesApi;
	readonly #notify: Notifier;

	constructor(title: string, api: FilesApi = sdkApi, notify: Notifier = toast) {
		this.#title = title;
		this.#api = api;
		this.#notify = notify;
	}

	cutoffOf(file: MediaFileResponse): CutoffState {
		return cutoffState(this.cutoff.get(file.id));
	}

	async load(): Promise<void> {
		try {
			const [files, statuses] = await Promise.all([
				this.#api.list(this.#title),
				this.#api.cutoff(this.#title)
			]);
			this.files = files;
			this.cutoff.clear();
			for (const status of statuses) this.cutoff.set(status.file, status);
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			if (this.status !== LoadStatus.READY) this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load the files.');
		}
	}

	askDelete(file: MediaFileResponse): void {
		this.confirmingDelete = file;
	}

	cancelDelete(): void {
		this.confirmingDelete = undefined;
	}

	async confirmDelete(): Promise<void> {
		const file = this.confirmingDelete;
		if (!file) return;
		this.confirmingDelete = undefined;
		this.deleting.add(file.id);
		try {
			await this.#api.remove(file.id);
			this.files = this.files.filter((f) => f.id !== file.id);
			this.#notify.success('File deleted', { description: file.path });
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not delete the file.'), {
				description: file.path
			});
		} finally {
			this.deleting.delete(file.id);
		}
	}

	/** Returns a cleanup. */
	watch(live: FilesLive): () => void {
		let renamed: ReturnType<typeof setTimeout> | undefined;
		const forTitle = (title: string) => {
			if (title === this.#title) void this.load();
		};
		const stops = [
			live.on(LibraryEvent.FILE_IMPORTED, (m) => forTitle((m.data as FileImported).title)),
			live.on(LibraryEvent.ITEM_AVAILABLE, (m) => forTitle((m.data as ItemAvailable).title)),
			live.on(LibraryEvent.FILE_DELETED, (m) => forTitle((m.data as FileDeleted).title)),
			live.on(CUTOFF_MET_EVENT, (m) => forTitle((m.data as CutoffMet).title)),
			live.on(LibraryEvent.FILE_TRANSCODED, (m) => {
				const file = (m.data as FileTranscoded).mediaFile;
				if (this.files.some((f) => f.id === file)) void this.load();
			}),
			live.on(EPISODE_GROUP_CHANGED_EVENT, (m) => {
				if ((m.data as EpisodeGroupChanged).title !== this.#title) return;
				clearTimeout(renamed);
				renamed = setTimeout(() => void this.load(), RENAME_SETTLE_MS);
			}),
			live.onReset(() => void this.load()),
			() => clearTimeout(renamed)
		];
		return () => stops.forEach((stop) => stop());
	}
}
