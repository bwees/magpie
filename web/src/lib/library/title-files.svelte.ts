import {
	apiErrorMessage,
	listMediaFiles,
	type EpisodeGroupChanged,
	type FileImported,
	type ItemAvailable,
	type MediaFileResponse
} from '$lib/sdk';
import type { LiveConnection } from '$lib/live/live-connection';
import { LoadStatus } from '$lib/load-status';
import { LibraryEvent } from './library';

export type FilesApi = (title: string) => Promise<MediaFileResponse[]>;

export type FilesLive = Pick<LiveConnection, 'on' | 'onReset'>;

export const EPISODE_GROUP_CHANGED_EVENT = 'catalog.EpisodeGroupChanged';
/** Library renames the files in a job after the renumbering commits, so the tab reloads a moment later. */
export const RENAME_SETTLE_MS = 1_500;

/** The title page's Files tab: the title's media files, reloaded when one is imported or Jellyfin confirms one. */
export class TitleFiles {
	files = $state<MediaFileResponse[]>([]);
	status = $state(LoadStatus.LOADING);
	error = $state('');

	/** Files Jellyfin still hasn't confirmed a day after their import. */
	get overdue(): MediaFileResponse[] {
		return this.files.filter((file) => file.jellyfinOverdue);
	}

	readonly #title: string;
	readonly #api: FilesApi;

	constructor(title: string, api: FilesApi = (id) => listMediaFiles({ title: id })) {
		this.#title = title;
		this.#api = api;
	}

	async load(): Promise<void> {
		try {
			this.files = await this.#api(this.#title);
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			if (this.status !== LoadStatus.READY) this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load the files.');
		}
	}

	/** Returns a cleanup. */
	watch(live: FilesLive): () => void {
		let renamed: ReturnType<typeof setTimeout> | undefined;
		const stops = [
			live.on(LibraryEvent.FILE_IMPORTED, (m) => {
				if ((m.data as FileImported).title === this.#title) void this.load();
			}),
			live.on(LibraryEvent.ITEM_AVAILABLE, (m) => {
				if ((m.data as ItemAvailable).title === this.#title) void this.load();
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
