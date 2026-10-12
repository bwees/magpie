import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	getSearchOptions,
	setSearchOptions,
	type SearchOptionsResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';

export interface SearchOptionsApi {
	get: () => Promise<SearchOptionsResponse>;
	set: (options: SearchOptionsResponse) => Promise<SearchOptionsResponse>;
}

export interface ErrorNotifier {
	error: (message: string) => void;
}

const sdkApi: SearchOptionsApi = {
	get: () => getSearchOptions(),
	set: (options) => setSearchOptions(options)
};

/** State behind Settings → Downloads "Search when a title is added", which saves on change. */
export class SearchOptionsManager {
	status = $state(LoadStatus.LOADING);
	error = $state('');
	saved = $state<SearchOptionsResponse>();
	saving = $state(false);

	readonly #api: SearchOptionsApi;
	readonly #notify: ErrorNotifier;

	constructor(api: SearchOptionsApi = sdkApi, notify: ErrorNotifier = toast) {
		this.#api = api;
		this.#notify = notify;
	}

	async load(): Promise<void> {
		try {
			this.saved = await this.#api.get();
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load the search settings.');
		}
	}

	async setSearchOnAdd(searchOnAdd: boolean): Promise<void> {
		this.saving = true;
		try {
			this.saved = await this.#api.set({ searchOnAdd });
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not save the search settings.'));
		} finally {
			this.saving = false;
		}
	}
}
