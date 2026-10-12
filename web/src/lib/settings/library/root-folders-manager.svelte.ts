import { SvelteSet } from 'svelte/reactivity';
import { toast } from 'svelte-sonner';
import {
	addRootFolder,
	apiErrorMessage,
	getHardlinkCheck,
	removeRootFolder,
	type AddRootFolderRequest,
	type HardlinkCheckResponse,
	type RootFolderResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import { MediaKind } from '$lib/library/library';

export interface RootFoldersApi {
	check: () => Promise<HardlinkCheckResponse>;
	add: (request: AddRootFolderRequest) => Promise<RootFolderResponse>;
	remove: (id: string) => Promise<unknown>;
}

export interface Notifier {
	success: (message: string, options?: { description?: string }) => void;
	error: (message: string, options?: { description?: string }) => void;
}

const sdkApi: RootFoldersApi = {
	check: () => getHardlinkCheck(),
	add: (request) => addRootFolder(request),
	remove: (id) => removeRootFolder(id)
};

/** State behind Settings → Library's root folders and hardlink check. */
export class RootFoldersManager {
	check = $state<HardlinkCheckResponse>();
	status = $state(LoadStatus.LOADING);
	error = $state('');
	newKind = $state(MediaKind.MOVIE);
	newPath = $state('');
	addError = $state<string>();
	adding = $state(false);
	readonly removing = new SvelteSet<string>();

	readonly #api: RootFoldersApi;
	readonly #notify: Notifier;

	constructor(api: RootFoldersApi = sdkApi, notify: Notifier = toast) {
		this.#api = api;
		this.#notify = notify;
	}

	get folders(): RootFolderResponse[] {
		return this.check?.rootFolders ?? [];
	}

	async load(): Promise<void> {
		try {
			this.check = await this.#api.check();
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			if (this.status !== LoadStatus.READY) this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load the root folders.');
		}
	}

	async add(): Promise<void> {
		const path = this.newPath.trim();
		if (!path.startsWith('/')) {
			this.addError = 'Enter an absolute path, such as /media/movies';
			return;
		}
		this.addError = undefined;
		this.adding = true;
		try {
			await this.#api.add({ kind: this.newKind, path });
			this.newPath = '';
			this.#notify.success('Root folder added', { description: path });
			await this.load();
		} catch (error) {
			this.addError = apiErrorMessage(error, 'Could not add the root folder.');
		} finally {
			this.adding = false;
		}
	}

	async remove(folder: RootFolderResponse): Promise<void> {
		this.removing.add(folder.id);
		try {
			await this.#api.remove(folder.id);
			if (this.check) {
				this.check.rootFolders = this.check.rootFolders.filter((f) => f.id !== folder.id);
			}
			this.#notify.success('Root folder removed', { description: folder.path });
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not remove the root folder.'), {
				description: folder.path
			});
		} finally {
			this.removing.delete(folder.id);
		}
	}
}
