import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	getAcquisitionSchedules,
	setAcquisitionSchedules,
	type SchedulesRequest,
	type SchedulesResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import { schedulesDraft, schedulesError, schedulesRequest, type SchedulesDraft } from './schedules';

export interface SchedulesApi {
	get: () => Promise<SchedulesResponse>;
	set: (request: SchedulesRequest) => Promise<SchedulesResponse>;
}

export interface Notifier {
	success: (message: string) => void;
	error: (message: string) => void;
}

const sdkApi: SchedulesApi = {
	get: () => getAcquisitionSchedules(),
	set: (request) => setAcquisitionSchedules(request)
};

/** State behind Settings → General → Schedules: RSS, the pending queue and the backlog sweeps. */
export class SchedulesManager {
	status = $state(LoadStatus.LOADING);
	error = $state('');
	saved = $state<SchedulesResponse>();
	draft = $state<SchedulesDraft>({
		rssIntervalMinutes: null,
		missingSweepDays: null,
		cutoffSweepDays: null
	});
	draftError = $state<string>();
	saving = $state(false);
	open = $state(false);

	readonly #api: SchedulesApi;
	readonly #notify: Notifier;

	constructor(api: SchedulesApi = sdkApi, notify: Notifier = toast) {
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
			this.error = apiErrorMessage(error, 'Could not load the schedules.');
		}
	}

	async save(): Promise<void> {
		this.draftError = schedulesError(this.draft);
		if (this.draftError) {
			return;
		}
		this.saving = true;
		try {
			this.#apply(await this.#api.set(schedulesRequest(this.draft)));
			this.#notify.success('Schedules saved');
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not save the schedules.'));
		} finally {
			this.saving = false;
		}
	}

	#apply(saved: SchedulesResponse): void {
		this.saved = saved;
		this.draft = schedulesDraft(saved);
	}
}
