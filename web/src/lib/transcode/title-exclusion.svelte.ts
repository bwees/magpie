import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	excludeFromTranscoding,
	includeInTranscoding,
	listTranscodeExclusions,
	type ExclusionResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import type { Notifier } from './gpu-jobs-manager.svelte';

/** Mirrors the Go `transcode.ExclusionScope` values. */
export enum ExclusionScope {
	TITLE = 'TITLE',
	ROOT_FOLDER = 'ROOT_FOLDER'
}

export interface ExclusionApi {
	list: () => Promise<ExclusionResponse[]>;
	exclude: (scope: ExclusionScope, ref: string) => Promise<unknown>;
	include: (scope: ExclusionScope, ref: string) => Promise<unknown>;
}

const sdkApi: ExclusionApi = {
	list: () => listTranscodeExclusions(),
	exclude: (scope, ref) => excludeFromTranscoding(scope, ref),
	include: (scope, ref) => includeInTranscoding(scope, ref)
};

/** The title page's "Exclude from transcoding" switch. */
export class TitleExclusion {
	excluded = $state(false);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	saving = $state(false);
	readonly #title: string;
	readonly #api: ExclusionApi;
	readonly #notify: Notifier;

	constructor(title: string, api: ExclusionApi = sdkApi, notify: Notifier = toast) {
		this.#title = title;
		this.#api = api;
		this.#notify = notify;
	}

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		try {
			const exclusions = await this.#api.list();
			this.excluded = exclusions.some(
				(e) => e.scope === ExclusionScope.TITLE && e.ref === this.#title
			);
			this.status = LoadStatus.READY;
		} catch (error) {
			this.error = apiErrorMessage(error, 'Could not load the transcode exclusion.');
			this.status = LoadStatus.ERROR;
		}
	}

	async set(excluded: boolean): Promise<void> {
		if (excluded === this.excluded) return;
		this.saving = true;
		try {
			const change = excluded ? this.#api.exclude : this.#api.include;
			await change(ExclusionScope.TITLE, this.#title);
			this.excluded = excluded;
			this.#notify.success(
				excluded ? 'Excluded from transcoding.' : 'Included in transcoding again.'
			);
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not change the transcode exclusion.'));
		} finally {
			this.saving = false;
		}
	}
}
