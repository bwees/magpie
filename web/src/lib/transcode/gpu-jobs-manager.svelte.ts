import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	cancelTranscodeJob,
	dismissTranscodeJob,
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
	JobListTab,
	LISTED_STATES,
	QUEUED_PAGE_SIZE,
	RUNNING_STATES,
	jobName,
	type JobDetail
} from './gpu-jobs';
import { SkipReason } from './transcode-settings';

export interface GpuJobsApi {
	list: () => Promise<JobListResponse>;
	cancel: (id: string) => Promise<JobResponse>;
	retry: (id: string) => Promise<JobResponse>;
	dismiss: (id: string) => Promise<unknown>;
}

export interface Notifier {
	success: (message: string) => void;
	error: (message: string) => void;
}

const sdkApi: GpuJobsApi = {
	list: () => listTranscodeJobs({ state: LISTED_STATES }),
	cancel: (id) => cancelTranscodeJob(id),
	retry: (id) => retryTranscodeJob(id),
	dismiss: (id) => dismissTranscodeJob(id)
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
	/** Eligible files kept as they are, per skip reason. */
	skipped = $state<Record<string, number>>({});
	status = $state(LoadStatus.LOADING);
	error = $state('');
	tab = $state(JobListTab.QUEUED);
	#queuedPage = $state(0);
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
		return this.jobs.filter((j) => j.state === JobState.FAILED);
	}

	get queuedPages(): number {
		return Math.max(1, Math.ceil(this.queued.length / QUEUED_PAGE_SIZE));
	}

	/** Zero-based, and kept in range as the queue shrinks. */
	get queuedPage(): number {
		return Math.min(this.#queuedPage, this.queuedPages - 1);
	}

	get queuedOnPage(): JobResponse[] {
		const start = this.queuedPage * QUEUED_PAGE_SIZE;
		return this.queued.slice(start, start + QUEUED_PAGE_SIZE);
	}

	nextQueuedPage(): void {
		this.#queuedPage = Math.min(this.queuedPage + 1, this.queuedPages - 1);
	}

	previousQueuedPage(): void {
		this.#queuedPage = Math.max(this.queuedPage - 1, 0);
	}

	/** Files that queue once an agent passes a test encode for the target. */
	get waitingForAgent(): number {
		return this.skipped[SkipReason.NO_CAPABLE_AGENT] ?? 0;
	}

	/** Skip reasons with files, in the order SKIP_LABELS lists them. */
	get skipReasons(): SkipReason[] {
		return Object.values(SkipReason).filter((reason) => (this.skipped[reason] ?? 0) > 0);
	}

	get skippedCount(): number {
		return this.skipReasons.reduce((total, reason) => total + this.skipped[reason], 0);
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
			this.skipped = list.skipped;
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

	dismiss(job: JobResponse): Promise<void> {
		return this.#act(job, async () => {
			await this.#api.dismiss(job.id);
			this.jobs = this.jobs.filter((j) => j.id !== job.id);
			this.#notify.success(`Dismissed ${jobName(job)}.`);
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
