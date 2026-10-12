import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	getSubtitleQuota,
	getSubtitleSettings,
	setSubtitleSettings,
	type QuotaResponse,
	type SubtitleSettingsBody
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import { settingsDraft, settingsError, settingsRequest, type SettingsDraft } from './subtitles';

export interface SubtitleSettingsApi {
	get: () => Promise<SubtitleSettingsBody>;
	set: (body: SubtitleSettingsBody) => Promise<SubtitleSettingsBody>;
	quota: () => Promise<QuotaResponse>;
}

export interface Notifier {
	success: (message: string) => void;
	error: (message: string) => void;
}

const sdkApi: SubtitleSettingsApi = {
	get: () => getSubtitleSettings(),
	set: (body) => setSubtitleSettings(body),
	quota: () => getSubtitleQuota()
};

/** State behind Settings → Subtitles: the Subtitles-owned settings and the OpenSubtitles quota badge. */
export class SubtitleSettings {
	status = $state(LoadStatus.LOADING);
	error = $state('');
	saved = $state<SubtitleSettingsBody>();
	draft = $state<SettingsDraft>();
	draftError = $state<string>();
	saving = $state(false);
	quota = $state<QuotaResponse>();

	readonly #api: SubtitleSettingsApi;
	readonly #notify: Notifier;

	constructor(api: SubtitleSettingsApi = sdkApi, notify: Notifier = toast) {
		this.#api = api;
		this.#notify = notify;
	}

	get dirty(): boolean {
		return (
			this.saved !== undefined &&
			this.draft !== undefined &&
			JSON.stringify(settingsDraft(this.saved)) !== JSON.stringify(this.draft)
		);
	}

	async load(): Promise<void> {
		try {
			const [saved, quota] = await Promise.all([this.#api.get(), this.#api.quota()]);
			this.#apply(saved);
			this.quota = quota;
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load the subtitle settings.');
		}
	}

	discard(): void {
		if (this.saved) this.#apply(this.saved);
	}

	async save(): Promise<void> {
		if (!this.draft) {
			return;
		}
		this.draftError = settingsError(this.draft);
		if (this.draftError) {
			return;
		}
		this.saving = true;
		try {
			this.#apply(await this.#api.set(settingsRequest(this.draft)));
			this.#notify.success('Subtitle settings saved');
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not save the subtitle settings.'));
		} finally {
			this.saving = false;
		}
	}

	#apply(saved: SubtitleSettingsBody): void {
		this.saved = saved;
		this.draft = settingsDraft(saved);
		this.draftError = undefined;
	}
}
