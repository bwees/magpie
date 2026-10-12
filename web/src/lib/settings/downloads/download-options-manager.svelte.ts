import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	getDownloadOptions,
	setDownloadOptions,
	type DownloadOptionsRequest,
	type DownloadOptionsResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import {
	optionsRequest,
	patternError,
	timeoutsChange,
	timeoutsDraft,
	timeoutsError,
	type TimeoutsDraft
} from './download-options';

export interface DownloadOptionsApi {
	get: () => Promise<DownloadOptionsResponse>;
	set: (request: DownloadOptionsRequest) => Promise<DownloadOptionsResponse>;
}

export interface Notifier {
	success: (message: string) => void;
	error: (message: string) => void;
}

const sdkApi: DownloadOptionsApi = {
	get: () => getDownloadOptions(),
	set: (request) => setDownloadOptions(request)
};

/** State behind Settings → Downloads. Patterns and the switch save at once; timeouts save with Save. */
export class DownloadOptionsManager {
	status = $state(LoadStatus.LOADING);
	error = $state('');
	saved = $state<DownloadOptionsResponse>();
	timeouts = $state<TimeoutsDraft>({ metadataTimeoutMinutes: null, stalledLimitHours: null });
	timeoutsError = $state<string>();
	newPattern = $state('');
	patternError = $state<string>();
	saving = $state(false);

	readonly #api: DownloadOptionsApi;
	readonly #notify: Notifier;

	constructor(api: DownloadOptionsApi = sdkApi, notify: Notifier = toast) {
		this.#api = api;
		this.#notify = notify;
	}

	async load(): Promise<void> {
		try {
			this.#apply(await this.#api.get());
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load the download settings.');
		}
	}

	async setEnabled(enabled: boolean): Promise<void> {
		await this.#save({ blockedFilesEnabled: enabled });
	}

	async addPattern(): Promise<void> {
		const pattern = this.newPattern.trim();
		this.patternError = patternError(pattern, this.saved?.blockedPatterns ?? []);
		if (this.patternError || !this.saved) {
			return;
		}
		if (await this.#save({ blockedPatterns: [...this.saved.blockedPatterns, pattern] })) {
			this.newPattern = '';
		}
	}

	async removePattern(pattern: string): Promise<void> {
		if (!this.saved) {
			return;
		}
		await this.#save({ blockedPatterns: this.saved.blockedPatterns.filter((p) => p !== pattern) });
	}

	async saveTimeouts(): Promise<void> {
		this.timeoutsError = timeoutsError(this.timeouts);
		if (this.timeoutsError) {
			return;
		}
		if (this.saved && (await this.#save(timeoutsChange(this.timeouts)))) {
			this.#apply(this.saved);
			this.#notify.success('Download timeouts saved');
		}
	}

	/** Keeps unsaved timeout edits; only the stored options change. */
	async #save(change: Partial<DownloadOptionsRequest>): Promise<boolean> {
		if (!this.saved) {
			return false;
		}
		this.saving = true;
		try {
			this.saved = await this.#api.set(optionsRequest(this.saved, change));
			return true;
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not save the download settings.'));
			return false;
		} finally {
			this.saving = false;
		}
	}

	#apply(options: DownloadOptionsResponse): void {
		this.saved = options;
		this.timeouts = timeoutsDraft(options);
	}
}
