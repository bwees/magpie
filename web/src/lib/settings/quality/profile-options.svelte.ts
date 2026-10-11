import { apiErrorMessage, listProfiles, type ProfileSummaryResponse } from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';

/** The profiles a picker offers, from GET /api/acquisition/profiles. */
export class ProfileOptions {
	status = $state(LoadStatus.LOADING);
	error = $state<string>();
	profiles = $state<ProfileSummaryResponse[]>([]);

	async load() {
		this.status = LoadStatus.LOADING;
		try {
			this.profiles = await listProfiles();
			this.error = undefined;
			this.status = LoadStatus.READY;
		} catch (error) {
			this.error = apiErrorMessage(error, 'Could not load profiles.');
			this.status = LoadStatus.ERROR;
		}
	}

	name(id: string | undefined): string | undefined {
		return this.profiles.find((profile) => profile.id === id)?.name;
	}
}
