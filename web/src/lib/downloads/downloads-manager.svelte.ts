import { SvelteSet } from 'svelte/reactivity';
import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	listDownloads,
	removeDownload,
	type DownloadProgressed,
	type DownloadResponse
} from '$lib/sdk';
import type { LiveConnection } from '$lib/live/live-connection';
import { LoadStatus } from '$lib/load-status';
import { DownloadEvent, nanosecondsToSeconds } from './downloads';

export interface DownloadsApi {
	list: () => Promise<DownloadResponse[]>;
	remove: (id: string, deleteData: boolean) => Promise<unknown>;
}

export interface Notifier {
	success: (message: string, options?: { description?: string }) => void;
	error: (message: string, options?: { description?: string }) => void;
}

const sdkApi: DownloadsApi = {
	list: () => listDownloads(),
	remove: (id, deleteData) => removeDownload(id, { deleteData })
};

export type DownloadsLive = Pick<LiveConnection, 'on' | 'onReset'>;

/** State and actions behind Activity → Downloads. Progress patches rows in place; grabs and endings reload. */
export class DownloadsManager {
	downloads = $state<DownloadResponse[]>([]);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	/** The download whose remove dialog is open. */
	confirming = $state<DownloadResponse | undefined>();
	deleteData = $state(true);
	readonly removing = new SvelteSet<string>();

	readonly #api: DownloadsApi;
	readonly #notify: Notifier;

	constructor(api: DownloadsApi = sdkApi, notify: Notifier = toast) {
		this.#api = api;
		this.#notify = notify;
	}

	async load(): Promise<void> {
		try {
			this.downloads = await this.#api.list();
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			if (this.status !== LoadStatus.READY) this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load downloads.');
		}
	}

	/** Returns a cleanup. */
	watch(live: DownloadsLive): () => void {
		const reload = () => void this.load();
		const stops = [
			live.on(DownloadEvent.PROGRESSED, (m) => this.#progress(m.data as DownloadProgressed)),
			live.on(DownloadEvent.GRABBED, reload),
			live.on(DownloadEvent.REJECTED, reload),
			live.on(DownloadEvent.COMPLETED, reload),
			live.on(DownloadEvent.FAILED, reload),
			live.onReset(reload)
		];
		return () => stops.forEach((stop) => stop());
	}

	askRemove(download: DownloadResponse): void {
		this.confirming = download;
		this.deleteData = true;
	}

	async confirmRemove(): Promise<void> {
		const download = this.confirming;
		if (!download) {
			return;
		}
		this.confirming = undefined;
		this.removing.add(download.id);
		try {
			await this.#api.remove(download.id, this.deleteData);
			this.downloads = this.downloads.filter((d) => d.id !== download.id);
			this.#notify.success('Download removed', { description: download.releaseName });
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not remove the download.'), {
				description: download.releaseName
			});
		} finally {
			this.removing.delete(download.id);
		}
	}

	#progress(event: DownloadProgressed): void {
		const row = this.downloads.find((d) => d.id === event.download);
		if (!row) {
			void this.load();
			return;
		}
		row.state = event.state;
		row.progress = event.progress;
		row.speedBps = event.speedBps;
		row.etaSeconds = nanosecondsToSeconds(event.eta);
		row.ratio = event.ratio;
	}
}
