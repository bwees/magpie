import { SvelteSet } from 'svelte/reactivity';
import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	listBlocklist,
	removeBlocklistEntry,
	type BlocklistEntryResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import { DownloadEvent } from './downloads';
import type { DownloadsLive, Notifier } from './downloads-manager.svelte';

export interface BlocklistApi {
	list: () => Promise<BlocklistEntryResponse[]>;
	remove: (id: string) => Promise<unknown>;
}

const sdkApi: BlocklistApi = {
	list: () => listBlocklist(),
	remove: (id) => removeBlocklistEntry(id)
};

/** State and actions behind Activity → Blocklist. */
export class BlocklistManager {
	entries = $state<BlocklistEntryResponse[]>([]);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	readonly removing = new SvelteSet<string>();

	readonly #api: BlocklistApi;
	readonly #notify: Notifier;

	constructor(api: BlocklistApi = sdkApi, notify: Notifier = toast) {
		this.#api = api;
		this.#notify = notify;
	}

	async load(): Promise<void> {
		try {
			this.entries = await this.#api.list();
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			if (this.status !== LoadStatus.READY) this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load the blocklist.');
		}
	}

	/** New entries come with every rejection. Returns a cleanup. */
	watch(live: DownloadsLive): () => void {
		const reload = () => void this.load();
		const stops = [live.on(DownloadEvent.REJECTED, reload), live.onReset(reload)];
		return () => stops.forEach((stop) => stop());
	}

	/** now is in epoch milliseconds. */
	isExpired(entry: BlocklistEntryResponse, now: number = Date.now()): boolean {
		return entry.expiresAt !== undefined && Date.parse(entry.expiresAt) <= now;
	}

	async remove(entry: BlocklistEntryResponse): Promise<void> {
		this.removing.add(entry.id);
		try {
			await this.#api.remove(entry.id);
			this.entries = this.entries.filter((e) => e.id !== entry.id);
			this.#notify.success('Removed from the blocklist', { description: entry.releaseName });
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not remove the entry.'), {
				description: entry.releaseName
			});
		} finally {
			this.removing.delete(entry.id);
		}
	}
}
