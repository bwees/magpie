import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	cancelTranscodeJob,
	listTranscodeJobs,
	retryTranscodeJob,
	type JobListResponse,
	type JobProgressed,
	type JobResponse
} from '$lib/sdk';
import type { LiveConnection } from '$lib/live/live-connection';
import { LoadStatus } from '$lib/load-status';
import { nanosecondsToSeconds } from '$lib/downloads/downloads';
import {
	JobEvent,
	JobState,
	LISTED_STATES,
	RETRYABLE_STATES,
	RUNNING_STATES,
	jobName,
	type JobDetail
} from './gpu-jobs';

export interface GpuJobsApi {
	list: () => Promise<JobListResponse>;
	cancel: (id: string) => Promise<JobResponse>;
	retry: (id: string) => Promise<JobResponse>;
}

export interface Notifier {
	success: (message: string) => void;
	error: (message: string) => void;
}

const sdkApi: GpuJobsApi = {
	list: () => listTranscodeJobs({ state: LISTED_STATES }),
	cancel: (id) => cancelTranscodeJob(id),
	retry: (id) => retryTranscodeJob(id)
};

export type GpuJobsLive = Pick<LiveConnection, 'on' | 'onReset'>;

/** Events after which the list is fetched again: a job appeared, started, ended or was swapped. */
const RELOAD_ON: JobEvent[] = [
	JobEvent.QUEUED,
	JobEvent.STARTED,
	JobEvent.FAILED,
	JobEvent.VERIFIED,
	JobEvent.SKIPPED,
	JobEvent.WHISPER_COMPLETED,
	JobEvent.FILE_TRANSCODED
];

/** State and actions behind Activity → GPU jobs. Progress patches rows in place; lifecycle events reload. */
export class GpuJobsManager {
	jobs = $state<JobResponse[]>([]);
	counts = $state<Record<string, number>>({});
	backlog = $state(0);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	/** Speed and ETA of running jobs, from their last progress event. */
	readonly details = new SvelteMap<string, JobDetail>();
	/** Jobs with an action in flight, so their buttons disable. */
	readonly busy = new SvelteSet<string>();

	readonly #api: GpuJobsApi;
	readonly #notify: Notifier;

	constructor(api: GpuJobsApi = sdkApi, notify: Notifier = toast) {
		this.#api = api;
		this.#notify = notify;
	}

	get running(): JobResponse[] {
		return this.jobs.filter((j) => RUNNING_STATES.includes(j.state));
	}

	get queued(): JobResponse[] {
		return this.jobs.filter((j) => j.state === JobState.QUEUED);
	}

	get failed(): JobResponse[] {
		return this.jobs.filter((j) => RETRYABLE_STATES.includes(j.state));
	}

	/** Every queued job, including those past the page the list holds. */
	get queuedCount(): number {
		return this.counts[JobState.QUEUED] ?? 0;
	}

	async load(): Promise<void> {
		try {
			const list = await this.#api.list();
			this.jobs = list.jobs;
			this.counts = list.counts;
			this.backlog = list.backlog;
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			if (this.status !== LoadStatus.READY) this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load GPU jobs.');
		}
	}

	/** Returns a cleanup. */
	watch(live: GpuJobsLive): () => void {
		const reload = () => void this.load();
		const stops = [
			live.on(JobEvent.PROGRESSED, (m) => this.#progress(m.data as JobProgressed)),
			...RELOAD_ON.map((event) => live.on(event, reload)),
			live.onReset(reload)
		];
		return () => stops.forEach((stop) => stop());
	}

	cancel(job: JobResponse): Promise<void> {
		return this.#act(job, async () => {
			await this.#api.cancel(job.id);
			this.jobs = this.jobs.filter((j) => j.id !== job.id);
			this.#notify.success(`Cancelled ${jobName(job)}.`);
		});
	}

	retry(job: JobResponse): Promise<void> {
		return this.#act(job, async () => {
			const updated = await this.#api.retry(job.id);
			this.jobs = this.jobs.map((j) => (j.id === updated.id ? updated : j));
			this.#notify.success(`Queued ${jobName(job)} again.`);
		});
	}

	async #act(job: JobResponse, action: () => Promise<void>): Promise<void> {
		this.busy.add(job.id);
		try {
			await action();
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, `Could not update ${jobName(job)}.`));
		} finally {
			this.busy.delete(job.id);
		}
	}

	#progress(event: JobProgressed): void {
		const row = this.jobs.find((j) => j.id === event.job);
		if (!row) {
			void this.load();
			return;
		}
		row.state = event.stage;
		row.progress = event.progress;
		this.details.set(event.job, {
			speed: event.speed,
			etaSeconds: nanosecondsToSeconds(event.eta)
		});
	}
}
