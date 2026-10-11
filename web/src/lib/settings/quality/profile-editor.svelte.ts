import { apiErrorMessage, updateProfile, type ProfileResponse } from '$lib/sdk';
import { profileDraft, profileDraftError, profileRequest, type ProfileDraft } from './quality';

/** Edits one profile's matrix: items, groups, sizes, cutoff and scores. */
export class ProfileEditor {
	profile: ProfileResponse;
	draft: ProfileDraft;
	error = $state<string>();
	saving = $state(false);

	readonly #onSaved: (profile: ProfileResponse) => void;

	constructor(profile: ProfileResponse, onSaved: (profile: ProfileResponse) => void = () => {}) {
		this.profile = $state(profile);
		this.draft = $state(profileDraft(profile));
		this.#onSaved = onSaved;
	}

	get isPreset(): boolean {
		return Boolean(this.profile.preset);
	}

	setCutoff(index: number) {
		this.draft.cutoff = index;
	}

	setAllowed(index: number, allowed: boolean) {
		this.draft.items[index].allowed = allowed;
	}

	reset() {
		this.draft = profileDraft(this.profile);
		this.error = undefined;
	}

	async save(): Promise<boolean> {
		this.error = profileDraftError(this.draft);
		if (this.error) {
			return false;
		}
		this.saving = true;
		try {
			this.profile = await updateProfile(this.profile.id, profileRequest(this.draft));
			this.draft = profileDraft(this.profile);
			this.#onSaved(this.profile);
			return true;
		} catch (error) {
			this.error = apiErrorMessage(error, 'Could not save the profile.');
			return false;
		} finally {
			this.saving = false;
		}
	}
}
