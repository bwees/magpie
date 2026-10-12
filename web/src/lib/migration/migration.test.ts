import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { HttpError } from '@oazapfts/runtime';
import type { MigrationResponse, MigrationReviews } from '$lib/sdk';
import {
	ArrApp,
	MIGRATION_PROGRESSED_EVENT,
	MigrationPhase,
	PathAccess,
	progressPercent,
	ReviewQueue,
	WizardStep
} from './migration';
import {
	MIGRATION_POLL_MS,
	MigrationWizard,
	PATH_CHECK_DEBOUNCE_MS,
	type MigrationApi,
	type MigrationLive
} from './migration-wizard.svelte';

const BAD_REQUEST = 400;

const SETUP: MigrationResponse = {
	phase: MigrationPhase.SETUP,
	sources: [],
	paths: [],
	cutoverConfirmed: false,
	options: { renameFolders: false, importAliases: false },
	progress: { step: '', done: 0, total: 0 },
	error: '',
	failedStep: '',
	canCommit: false
};

const CONNECTED: MigrationResponse = {
	...SETUP,
	sources: [
		{
			app: ArrApp.SONARR,
			url: 'http://sonarr',
			version: '4.0.20',
			skipped: false,
			rootFolders: ['/tv']
		},
		{ app: ArrApp.RADARR, url: '', version: '', skipped: true, rootFolders: [] }
	],
	paths: [{ app: ArrApp.SONARR, from: '/tv', to: '/tv', access: PathAccess.MISSING }]
};

function reviews(pending: number): MigrationReviews {
	return {
		queues: [{ queue: ReviewQueue.CLASHES, total: 1, pending }],
		titles: [],
		numbering: [],
		clashes: [],
		specs: [],
		seriesTypes: []
	};
}

const REPORT = {
	apps: [],
	numberingMismatches: 0,
	unmappedEpisodes: 0,
	formats: 0,
	formatClashes: 1,
	untranslatedSpecs: 0,
	releaseProfileFormats: 0,
	profiles: 0,
	profileClashes: 0,
	files: 0,
	missingFiles: 0,
	blocklistWithHash: 0,
	blocklistWithoutHash: 0,
	history: 0,
	warnings: []
};

const PLANNED: MigrationResponse = {
	...CONNECTED,
	phase: MigrationPhase.REVIEW,
	report: REPORT,
	reviews: reviews(0)
};

function fakeApi(states: MigrationResponse[]): MigrationApi {
	const next = () => states.shift() ?? SETUP;
	return {
		state: vi.fn(async () => next()),
		connect: vi.fn(async () => next()),
		setPaths: vi.fn(async () => next()),
		checkPath: vi.fn(async (path: string) => ({ path, access: PathAccess.OK })),
		setCutover: vi.fn(async () => next()),
		setOptions: vi.fn(async () => next()),
		dryRun: vi.fn(async () => next()),
		decide: vi.fn(async () => next()),
		skip: vi.fn(async () => next()),
		search: vi.fn(async () => []),
		commit: vi.fn(async () => next()),
		reset: vi.fn(async () => undefined)
	};
}

function fakeLive(): MigrationLive & { emit: (data: unknown) => void } {
	let handler: ((message: { data: unknown }) => void) | undefined;
	return {
		on: (type, fn) => {
			if (type === MIGRATION_PROGRESSED_EVENT) handler = fn as typeof handler;
			return () => (handler = undefined);
		},
		onReset: () => () => undefined,
		emit: (data) => handler?.({ data })
	};
}

describe('MigrationWizard', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	it('opens at Connect and keeps later steps shut until an app is connected', async () => {
		const wizard = new MigrationWizard(fakeApi([SETUP]));
		await wizard.load();

		expect(wizard.step).toBe(WizardStep.CONNECT);
		expect(wizard.canVisit(WizardStep.PATHS)).toBe(false);
		expect(wizard.canVisit(WizardStep.REVIEW)).toBe(false);
	});

	it('moves to Paths once both apps are connected or skipped, and fills the path drafts', async () => {
		const api = fakeApi([SETUP, CONNECTED]);
		const wizard = new MigrationWizard(api);
		await wizard.load();
		wizard.forms[ArrApp.SONARR].url = 'http://sonarr';
		wizard.forms[ArrApp.SONARR].apiKey = 'key';

		await wizard.connect(ArrApp.SONARR);

		expect(api.connect).toHaveBeenCalledWith({
			app: ArrApp.SONARR,
			url: 'http://sonarr',
			apiKey: 'key',
			skip: false
		});
		expect(wizard.forms[ArrApp.SONARR].apiKey).toBe('');
		expect(wizard.step).toBe(WizardStep.PATHS);
		expect(wizard.pathDraft(ArrApp.SONARR, '/tv')).toBe('/tv');
		expect(wizard.access(ArrApp.SONARR, '/tv')).toBe(PathAccess.MISSING);
	});

	it('shows why a connection failed on its form', async () => {
		const api = fakeApi([SETUP]);
		vi.mocked(api.connect).mockRejectedValueOnce(
			new HttpError(
				BAD_REQUEST,
				{ detail: 'Sonarr 3.0.10; Magpie imports from Sonarr 4.0 or later' },
				new Headers()
			)
		);
		const wizard = new MigrationWizard(api);
		await wizard.load();

		await wizard.connect(ArrApp.SONARR);

		expect(wizard.forms[ArrApp.SONARR].error).toContain('Sonarr 4.0');
	});

	it('checks a path once the typing stops', async () => {
		const api = fakeApi([CONNECTED]);
		const wizard = new MigrationWizard(api);
		await wizard.load();

		wizard.editPath(ArrApp.SONARR, '/tv', '/data');
		wizard.editPath(ArrApp.SONARR, '/tv', '/data/tv');
		await vi.advanceTimersByTimeAsync(PATH_CHECK_DEBOUNCE_MS);

		expect(api.checkPath).toHaveBeenCalledTimes(1);
		expect(api.checkPath).toHaveBeenCalledWith('/data/tv');
		expect(wizard.access(ArrApp.SONARR, '/tv')).toBe(PathAccess.OK);
		expect(wizard.pathsReachable).toBe(true);
	});

	// Migration seed 9.
	it('keeps Commit disabled until the cutover checkbox is ticked', async () => {
		const ticked = { ...PLANNED, cutoverConfirmed: true, canCommit: true };
		const api = fakeApi([PLANNED, ticked]);
		const wizard = new MigrationWizard(api);
		await wizard.load();

		expect(wizard.canCommit).toBe(false);
		await wizard.startCommit();
		expect(api.commit).not.toHaveBeenCalled();

		await wizard.setCutover(true);
		expect(wizard.canCommit).toBe(true);
	});

	it('lands on Review while a queue is open and settles it with Skip', async () => {
		const open = { ...PLANNED, reviews: reviews(1) };
		const api = fakeApi([open, PLANNED]);
		const wizard = new MigrationWizard(api);
		await wizard.load();
		expect(wizard.step).toBe(WizardStep.REVIEW);
		expect(wizard.pendingReviews).toBe(1);

		await wizard.skipQueue(ReviewQueue.CLASHES);

		expect(api.skip).toHaveBeenCalledWith(ReviewQueue.CLASHES);
		expect(wizard.pendingReviews).toBe(0);
	});

	it('moves the bar on live progress and reloads when the phase changes', async () => {
		const running = { ...CONNECTED, phase: MigrationPhase.DRY_RUN_RUNNING };
		const api = fakeApi([running, PLANNED]);
		const live = fakeLive();
		const wizard = new MigrationWizard(api);
		await wizard.load();
		const stop = wizard.watch(live);
		expect(wizard.step).toBe(WizardStep.DRY_RUN);

		live.emit({ phase: MigrationPhase.DRY_RUN_RUNNING, step: 'MATCHING', done: 3, total: 4 });
		expect(wizard.state?.progress).toEqual({ step: 'MATCHING', done: 3, total: 4 });
		expect(api.state).toHaveBeenCalledTimes(1);

		live.emit({ phase: MigrationPhase.REVIEW, step: '', done: 0, total: 0 });
		await vi.waitFor(() => expect(wizard.phase).toBe(MigrationPhase.REVIEW));
		stop();
	});

	it('polls a running job when no live connection reports it', async () => {
		const running = {
			...CONNECTED,
			phase: MigrationPhase.COMMIT_RUNNING,
			report: REPORT,
			reviews: reviews(0)
		};
		const done = { ...running, phase: MigrationPhase.DONE };
		const api = fakeApi([running, running, done]);
		const wizard = new MigrationWizard(api);
		await wizard.load();
		const stop = wizard.watch();

		await vi.advanceTimersByTimeAsync(MIGRATION_POLL_MS * 2);

		expect(wizard.phase).toBe(MigrationPhase.DONE);
		expect(wizard.step).toBe(WizardStep.COMMIT);
		expect(wizard.canVisit(WizardStep.CONNECT)).toBe(false);
		stop();
	});

	it('reports progress as a percent once the total is known', () => {
		expect(progressPercent(0, 0)).toBeUndefined();
		expect(progressPercent(1, 3)).toBe(33);
		expect(progressPercent(5, 4)).toBe(100);
	});
});
