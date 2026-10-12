import {
	apiErrorMessage,
	checkMigrationPath,
	connectMigrationSource,
	decideMigrationReview,
	getMigration,
	resetMigration,
	searchMigrationTitle,
	setMigrationCutover,
	setMigrationOptions,
	setMigrationPaths,
	skipMigrationReviewQueue,
	startMigrationCommit,
	startMigrationDryRun,
	type MigrationCandidate,
	type MigrationConnectRequest,
	type MigrationDecisionRequest,
	type MigrationResponse,
	type MigrationOptionsRequest,
	type MigrationPathRequest,
	type MigrationPathCheck
} from '$lib/sdk';
import type { LiveConnection } from '$lib/live/live-connection';
import { LoadStatus } from '$lib/load-status';
import {
	ARR_APPS,
	ArrApp,
	MIGRATION_PROGRESSED_EVENT,
	MigrationPhase,
	PathAccess,
	ReviewQueue,
	RUNNING_PHASES,
	WIZARD_STEPS,
	WizardStep,
	type ProgressEvent
} from './migration';

export interface MigrationApi {
	state: () => Promise<MigrationResponse>;
	connect: (request: MigrationConnectRequest) => Promise<MigrationResponse>;
	setPaths: (paths: MigrationPathRequest[]) => Promise<MigrationResponse>;
	checkPath: (path: string) => Promise<MigrationPathCheck>;
	setCutover: (confirmed: boolean) => Promise<MigrationResponse>;
	setOptions: (options: MigrationOptionsRequest) => Promise<MigrationResponse>;
	dryRun: () => Promise<MigrationResponse>;
	decide: (decision: MigrationDecisionRequest) => Promise<MigrationResponse>;
	skip: (queue: ReviewQueue) => Promise<MigrationResponse>;
	search: (kind: string, query: string) => Promise<MigrationCandidate[]>;
	commit: () => Promise<MigrationResponse>;
	reset: () => Promise<unknown>;
}

const sdkApi: MigrationApi = {
	state: () => getMigration(),
	connect: (request) => connectMigrationSource(request),
	setPaths: (paths) => setMigrationPaths({ paths }),
	checkPath: (path) => checkMigrationPath({ path }),
	setCutover: (confirmed) => setMigrationCutover({ confirmed }),
	setOptions: (options) => setMigrationOptions(options),
	dryRun: () => startMigrationDryRun(),
	decide: (decision) => decideMigrationReview(decision),
	skip: (queue) => skipMigrationReviewQueue({ queue }),
	search: (kind, query) => searchMigrationTitle(kind, query),
	commit: () => startMigrationCommit(),
	reset: () => resetMigration()
};

export type MigrationLive = Pick<LiveConnection, 'on' | 'onReset'>;

/** How often the wizard refreshes a running job when no live connection tells it. */
export const MIGRATION_POLL_MS = 1_500;
/** How long a path stays unchanged before its live access check runs. */
export const PATH_CHECK_DEBOUNCE_MS = 400;

/** One app's connection form. */
export class ConnectionForm {
	url = $state('');
	apiKey = $state('');
	busy = $state(false);
	error = $state('');
}

/** The TMDB search and split picks of one unmatched title. */
export class TitleSearch {
	query = $state('');
	results = $state<MigrationCandidate[]>([]);
	searching = $state(false);
	error = $state('');
	/** TMDB IDs picked to split a series into. */
	picked = $state<number[]>([]);
}

function pathKey(app: string, from: string): string {
	return `${app}\u0000${from}`;
}

/**
 * The Sonarr/Radarr import wizard: the server's migration state, the step on screen, and every action the
 * steps take. The server owns the state; each action replaces it with the server's answer.
 */
export class MigrationWizard {
	state = $state<MigrationResponse>();
	status = $state(LoadStatus.LOADING);
	error = $state('');
	step = $state(WizardStep.CONNECT);
	/** The action in flight, so its button shows it and the rest wait. */
	busy = $state(false);
	actionError = $state('');
	readonly forms: Record<ArrApp, ConnectionForm> = {
		[ArrApp.SONARR]: new ConnectionForm(),
		[ArrApp.RADARR]: new ConnectionForm()
	};
	/** Edited "server sees" paths by app and app path, and their live access checks. */
	pathDrafts = $state<Record<string, string>>({});
	pathAccess = $state<Record<string, string>>({});
	readonly searches: Record<string, TitleSearch> = $state({});

	readonly #api: MigrationApi;
	#poll: ReturnType<typeof setTimeout> | undefined;
	#checks: Record<string, ReturnType<typeof setTimeout>> = {};
	#watching = false;
	#chosenStep = false;

	constructor(api: MigrationApi = sdkApi) {
		this.#api = api;
	}

	get phase(): string {
		return this.state?.phase ?? MigrationPhase.SETUP;
	}

	get running(): boolean {
		return RUNNING_PHASES.includes(this.phase);
	}

	get connected(): boolean {
		return (this.state?.sources ?? []).some((s) => !s.skipped);
	}

	get planned(): boolean {
		return this.state?.report !== undefined;
	}

	get committed(): boolean {
		return (
			this.phase === MigrationPhase.COMMIT_RUNNING ||
			this.phase === MigrationPhase.COMMIT_FAILED ||
			this.phase === MigrationPhase.DONE
		);
	}

	get pendingReviews(): number {
		return (this.state?.reviews?.queues ?? []).reduce((sum, q) => sum + q.pending, 0);
	}

	/** Commit needs a settled review, the cutover checkbox and no running job; the server decides the rest. */
	get canCommit(): boolean {
		return (this.state?.canCommit ?? false) && !this.running && !this.busy;
	}

	canVisit(step: WizardStep): boolean {
		switch (step) {
			case WizardStep.CONNECT:
				return !this.committed;
			case WizardStep.PATHS:
			case WizardStep.CUTOVER:
			case WizardStep.DRY_RUN:
				return this.connected && !this.committed;
			case WizardStep.REVIEW:
			case WizardStep.OPTIONS:
				return this.planned && !this.committed;
			case WizardStep.COMMIT:
				return this.planned || this.committed;
		}
	}

	visit(step: WizardStep): void {
		if (!this.canVisit(step)) return;
		this.step = step;
		this.#chosenStep = true;
		this.actionError = '';
	}

	/** The next step after the current one, if it can be visited. */
	next(): void {
		const index = WIZARD_STEPS.findIndex((s) => s.step === this.step);
		const following = WIZARD_STEPS[index + 1];
		if (following) this.visit(following.step);
	}

	async load(): Promise<void> {
		try {
			this.#apply(await this.#api.state());
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			if (this.status !== LoadStatus.READY) this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load the import.');
		}
	}

	/** Follows progress through live events, or by polling without them. Returns a cleanup. */
	watch(live?: MigrationLive): () => void {
		this.#watching = true;
		const stops: (() => void)[] = [];
		if (live) {
			stops.push(
				live.on(MIGRATION_PROGRESSED_EVENT, (message) =>
					this.progressed(message.data as ProgressEvent)
				),
				live.onReset(() => void this.load())
			);
		}
		this.#schedule();
		return () => {
			this.#watching = false;
			clearTimeout(this.#poll);
			for (const timer of Object.values(this.#checks)) clearTimeout(timer);
			for (const stop of stops) stop();
		};
	}

	/** A progress event: the bar moves at once, and a phase change reloads the state. */
	progressed(progress: ProgressEvent): void {
		if (!this.state) return;
		if (progress.phase !== this.state.phase) {
			void this.load();
			return;
		}
		this.state.progress = { step: progress.step, done: progress.done, total: progress.total };
	}

	async connect(app: ArrApp): Promise<void> {
		const form = this.forms[app];
		form.busy = true;
		form.error = '';
		try {
			this.#apply(
				await this.#api.connect({
					app,
					url: form.url.trim(),
					apiKey: form.apiKey.trim(),
					skip: false
				})
			);
			form.apiKey = '';
		} catch (error) {
			form.error = apiErrorMessage(error, `Could not reach ${app.toLowerCase()}.`);
		} finally {
			form.busy = false;
		}
	}

	async skipApp(app: ArrApp): Promise<void> {
		const form = this.forms[app];
		form.busy = true;
		form.error = '';
		try {
			this.#apply(await this.#api.connect({ app, url: '', apiKey: '', skip: true }));
		} catch (error) {
			form.error = apiErrorMessage(error);
		} finally {
			form.busy = false;
		}
	}

	source(app: ArrApp) {
		return this.state?.sources.find((s) => s.app === app);
	}

	pathDraft(app: string, from: string): string {
		return this.pathDrafts[pathKey(app, from)] ?? '';
	}

	access(app: string, from: string): string {
		return this.pathAccess[pathKey(app, from)] ?? '';
	}

	/** Edits where the server sees a root folder and checks it once the typing stops. */
	editPath(app: string, from: string, to: string): void {
		const key = pathKey(app, from);
		this.pathDrafts[key] = to;
		clearTimeout(this.#checks[key]);
		this.#checks[key] = setTimeout(() => void this.#check(key, to), PATH_CHECK_DEBOUNCE_MS);
	}

	async #check(key: string, path: string): Promise<void> {
		try {
			const result = await this.#api.checkPath(path);
			if (this.pathDrafts[key] === path) this.pathAccess[key] = result.access;
		} catch {
			delete this.pathAccess[key];
		}
	}

	get pathsReachable(): boolean {
		return (this.state?.paths ?? []).every((p) => this.access(p.app, p.from) === PathAccess.OK);
	}

	async savePaths(): Promise<boolean> {
		const paths = (this.state?.paths ?? []).map((p) => ({
			app: p.app,
			from: p.from,
			to: this.pathDraft(p.app, p.from)
		}));
		return this.#act(() => this.#api.setPaths(paths), 'Could not save the paths.');
	}

	async setCutover(confirmed: boolean): Promise<void> {
		await this.#act(() => this.#api.setCutover(confirmed), 'Could not save the checklist.');
	}

	async setOption(option: keyof MigrationOptionsRequest, on: boolean): Promise<void> {
		if (!this.state) return;
		const options = { ...this.state.options, [option]: on };
		await this.#act(() => this.#api.setOptions(options), 'Could not save the option.');
	}

	async startDryRun(): Promise<void> {
		if (await this.#act(() => this.#api.dryRun(), 'Could not start the dry run.')) this.#schedule();
	}

	async decide(decision: MigrationDecisionRequest): Promise<void> {
		await this.#act(() => this.#api.decide(decision), 'Could not save that decision.');
	}

	async skipQueue(queue: ReviewQueue): Promise<void> {
		await this.#act(() => this.#api.skip(queue), 'Could not settle the queue.');
	}

	search(id: string): TitleSearch {
		this.searches[id] ??= new TitleSearch();
		return this.searches[id];
	}

	async runSearch(id: string, kind: string): Promise<void> {
		const search = this.search(id);
		if (search.query.trim() === '') return;
		search.searching = true;
		search.error = '';
		try {
			search.results = await this.#api.search(kind, search.query.trim());
		} catch (error) {
			search.error = apiErrorMessage(error, 'TMDB search failed.');
		} finally {
			search.searching = false;
		}
	}

	togglePick(id: string, tmdb: number): void {
		const search = this.search(id);
		search.picked = search.picked.includes(tmdb)
			? search.picked.filter((t) => t !== tmdb)
			: [...search.picked, tmdb];
	}

	async startCommit(): Promise<void> {
		if (!this.canCommit) return;
		if (await this.#act(() => this.#api.commit(), 'Could not start the import.')) this.#schedule();
	}

	async reset(): Promise<void> {
		this.busy = true;
		try {
			await this.#api.reset();
			this.#chosenStep = false;
			this.pathDrafts = {};
			this.pathAccess = {};
			await this.load();
		} catch (error) {
			this.actionError = apiErrorMessage(error, 'Could not start over.');
		} finally {
			this.busy = false;
		}
	}

	async #act(call: () => Promise<MigrationResponse>, fallback: string): Promise<boolean> {
		this.busy = true;
		this.actionError = '';
		try {
			this.#apply(await call());
			return true;
		} catch (error) {
			this.actionError = apiErrorMessage(error, fallback);
			return false;
		} finally {
			this.busy = false;
		}
	}

	#apply(state: MigrationResponse): void {
		this.state = state;
		for (const p of state.paths) {
			const key = pathKey(p.app, p.from);
			if (!(key in this.pathDrafts)) {
				this.pathDrafts[key] = p.to;
				this.pathAccess[key] = p.access;
			}
		}
		for (const review of state.reviews?.titles ?? []) {
			this.searches[review.id] ??= new TitleSearch();
		}
		for (const source of state.sources) {
			const form = this.forms[source.app as ArrApp];
			if (form && form.url === '' && !source.skipped) form.url = source.url;
		}
		if (!this.#chosenStep || !this.canVisit(this.step)) this.step = this.#stepForPhase();
		this.#schedule();
	}

	/** Where a returning admin lands: the step the import is at. */
	#stepForPhase(): WizardStep {
		switch (this.phase) {
			case MigrationPhase.DRY_RUN_RUNNING:
			case MigrationPhase.DRY_RUN_FAILED:
				return WizardStep.DRY_RUN;
			case MigrationPhase.REVIEW:
				return this.pendingReviews > 0 ? WizardStep.REVIEW : WizardStep.COMMIT;
			case MigrationPhase.COMMIT_RUNNING:
			case MigrationPhase.COMMIT_FAILED:
			case MigrationPhase.DONE:
				return WizardStep.COMMIT;
		}
		return this.connected && ARR_APPS.every((app) => this.source(app))
			? WizardStep.PATHS
			: WizardStep.CONNECT;
	}

	#schedule(): void {
		clearTimeout(this.#poll);
		if (this.#watching && this.running)
			this.#poll = setTimeout(() => void this.load(), MIGRATION_POLL_MS);
	}
}
