import {
	apiErrorMessage,
	blockTerm,
	createProfileFromPreset,
	deleteCustomFormat,
	deleteProfile,
	getAcquisitionSettings,
	getProfile,
	getTrashStatus,
	listCustomFormats,
	listProfiles,
	syncTrash,
	updateAcquisitionSettings,
	type CustomFormatResponse,
	type ProfileSummaryResponse,
	type TrashStatusResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import { ProfileEditor } from './profile-editor.svelte';
import {
	presetProfile,
	settingsDraft,
	settingsDraftError,
	settingsRequest,
	type QualityPreset,
	type SettingsDraft
} from './quality';

/** How long "Sync now" waits for the queued sync to report back, and how often it asks. */
export const SYNC_WAIT_MS = 120_000;
export const SYNC_POLL_MS = 2_000;

export type Sleep = (ms: number) => Promise<void>;

const sleep: Sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

/** State for Settings → Quality: presets, defaults, TRaSH sync, block terms and the Advanced matrix. */
export class QualitySettingsManager {
	status = $state(LoadStatus.LOADING);
	error = $state<string>();

	profiles = $state<ProfileSummaryResponse[]>([]);
	formats = $state<CustomFormatResponse[]>([]);

	settings = $state<SettingsDraft>();
	settingsError = $state<string>();
	savingSettings = $state(false);

	trash = $state<TrashStatusResponse>();
	syncing = $state(false);
	trashError = $state<string>();

	creatingPreset = $state<QualityPreset>();
	presetError = $state<string>();

	term = $state('');
	blocking = $state(false);
	blockError = $state<string>();

	formatQuery = $state('');
	advancedOpen = $state(false);
	editor = $state<ProfileEditor>();
	editorLoading = $state(false);
	editorError = $state<string>();

	readonly #sleep: Sleep;
	readonly #notify: (message: string) => void;

	constructor(notify: (message: string) => void = () => {}, wait: Sleep = sleep) {
		this.#notify = notify;
		this.#sleep = wait;
	}

	get defaultProfileId(): string | undefined {
		return this.settings?.defaultProfileId;
	}

	/** Formats whose name contains the filter text, ignoring case. */
	get filteredFormats(): CustomFormatResponse[] {
		const query = this.formatQuery.trim().toLowerCase();
		return this.formats.filter((format) => format.name.toLowerCase().includes(query));
	}

	presetProfile(preset: QualityPreset): ProfileSummaryResponse | undefined {
		return presetProfile(this.profiles, preset);
	}

	async load() {
		this.status = LoadStatus.LOADING;
		try {
			const [profiles, settings, trash, formats] = await Promise.all([
				listProfiles(),
				getAcquisitionSettings(),
				getTrashStatus(),
				listCustomFormats()
			]);
			this.profiles = profiles;
			this.settings = settingsDraft(settings);
			this.trash = trash;
			this.formats = formats;
			this.error = undefined;
			this.status = LoadStatus.READY;
		} catch (error) {
			this.error = apiErrorMessage(error, 'Could not load quality settings.');
			this.status = LoadStatus.ERROR;
		}
	}

	async createPreset(preset: QualityPreset) {
		this.creatingPreset = preset;
		this.presetError = undefined;
		try {
			const profile = await createProfileFromPreset({ preset });
			this.profiles = await listProfiles();
			this.#notify(`Created the ${profile.name} profile`);
		} catch (error) {
			this.presetError = apiErrorMessage(error, 'Could not create the preset.');
		} finally {
			this.creatingPreset = undefined;
		}
	}

	async makeDefault(profileId: string) {
		if (!this.settings) {
			return;
		}
		this.settings.defaultProfileId = profileId;
		await this.saveSettings();
	}

	async setTrashSync(enabled: boolean) {
		if (!this.settings) {
			return;
		}
		this.settings.trashSyncEnabled = enabled;
		if (await this.saveSettings()) {
			this.trash = await getTrashStatus();
		}
	}

	async saveSettings(): Promise<boolean> {
		if (!this.settings) {
			return false;
		}
		this.settingsError = settingsDraftError(this.settings);
		if (this.settingsError) {
			return false;
		}
		this.savingSettings = true;
		try {
			this.settings = settingsDraft(
				await updateAcquisitionSettings(settingsRequest(this.settings))
			);
			return true;
		} catch (error) {
			this.settingsError = apiErrorMessage(error, 'Could not save.');
			return false;
		} finally {
			this.savingSettings = false;
		}
	}

	/** Queues a sync, then waits until the server records an attempt newer than the one it had. */
	async syncNow() {
		this.syncing = true;
		this.trashError = undefined;
		try {
			const before = (await syncTrash()).lastAttemptAt;
			const deadline = Date.now() + SYNC_WAIT_MS;
			while (Date.now() < deadline) {
				await this.#sleep(SYNC_POLL_MS);
				const status = await getTrashStatus();
				if (status.lastAttemptAt && status.lastAttemptAt !== before) {
					this.trash = status;
					this.trashError = status.lastError || undefined;
					if (!status.lastError) {
						this.formats = await listCustomFormats();
						this.#notify('TRaSH Guides synced');
					}
					return;
				}
			}
			this.trashError = 'The sync is still running. Check back in a minute.';
		} catch (error) {
			this.trashError = apiErrorMessage(error, 'Could not start the sync.');
		} finally {
			this.syncing = false;
		}
	}

	async addBlockTerm() {
		const term = this.term.trim();
		if (!term) {
			this.blockError = 'Type a word or phrase to block';
			return;
		}
		this.blocking = true;
		this.blockError = undefined;
		try {
			await blockTerm({ term });
			this.term = '';
			this.formats = await listCustomFormats();
			this.#notify(`Releases containing “${term}” won't be grabbed`);
		} catch (error) {
			this.blockError = apiErrorMessage(error, 'Could not block the term.');
		} finally {
			this.blocking = false;
		}
	}

	async removeFormat(id: string) {
		try {
			await deleteCustomFormat(id);
			this.formats = this.formats.filter((format) => format.id !== id);
		} catch (error) {
			this.blockError = apiErrorMessage(error, 'Could not delete the format.');
		}
	}

	async openProfile(id: string) {
		this.editorLoading = true;
		this.editorError = undefined;
		try {
			this.editor = new ProfileEditor(await getProfile(id), () => this.#refreshProfiles());
		} catch (error) {
			this.editorError = apiErrorMessage(error, 'Could not load the profile.');
		} finally {
			this.editorLoading = false;
		}
	}

	async removeProfile(id: string) {
		this.editorError = undefined;
		try {
			await deleteProfile(id);
			this.editor = undefined;
			await this.#refreshProfiles();
		} catch (error) {
			this.editorError = apiErrorMessage(error, 'Could not delete the profile.');
		}
	}

	async #refreshProfiles() {
		this.profiles = await listProfiles();
	}
}
