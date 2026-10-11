import {
	apiErrorMessage,
	createProfileFromPreset,
	getAcquisitionSettings,
	listProfiles,
	syncTrash,
	updateAcquisitionSettings,
	type AcquisitionSettingsResponse,
	type ProfileSummaryResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import { presetProfile, QualityPreset } from './quality';

/** The first-run "Default quality preset" step: pick 720p, 1080p or 2160p for new requests. */
export class PresetStep {
	status = $state(LoadStatus.LOADING);
	error = $state<string>();
	selected = $state(QualityPreset.P1080);
	saving = $state(false);

	#profiles: ProfileSummaryResponse[] = [];
	#settings?: AcquisitionSettingsResponse;

	async load() {
		this.status = LoadStatus.LOADING;
		try {
			[this.#profiles, this.#settings] = await Promise.all([
				listProfiles(),
				getAcquisitionSettings()
			]);
			const current = this.#profiles.find(
				(profile) => profile.id === this.#settings?.defaultProfileId
			);
			if (current?.preset) {
				this.selected = current.preset as QualityPreset;
			}
			this.error = undefined;
			this.status = LoadStatus.READY;
		} catch (error) {
			this.error = apiErrorMessage(error, 'Could not load the presets.');
			this.status = LoadStatus.ERROR;
		}
	}

	/**
	 * Makes the selected preset the default, creating it if it was deleted, and starts a TRaSH sync
	 * so the presets get their format scores. Returns false when it couldn't save.
	 */
	async save(): Promise<boolean> {
		if (!this.#settings) {
			return false;
		}
		this.saving = true;
		this.error = undefined;
		try {
			const profile =
				presetProfile(this.#profiles, this.selected) ??
				(await createProfileFromPreset({ preset: this.selected }));
			this.#settings = await updateAcquisitionSettings({
				...this.#settings,
				defaultProfileId: profile.id
			});
			await syncTrash().catch(() => undefined);
			return true;
		} catch (error) {
			this.error = apiErrorMessage(error, 'Could not save the default preset.');
			return false;
		} finally {
			this.saving = false;
		}
	}
}
