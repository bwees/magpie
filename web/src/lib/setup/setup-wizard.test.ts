import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ConnectionsResponse, MeResponse } from '$lib/sdk';
import { Service } from '$lib/connections/services';
import { SetupWizard } from './setup-wizard.svelte';
import { StepOutcome, WIZARD_STEPS, WizardStep } from './steps';

const ADMIN: MeResponse = {
	id: '01JADMIN',
	username: 'admin',
	displayName: 'admin',
	role: 'ADMIN',
	localAdmin: true,
	permissions: {
		autoApproveMovies: true,
		autoApproveTv: true,
		reportIssues: true,
		viewOthersRequests: true,
		manageFiles: true
	}
};
const EMPTY_CONNECTIONS: ConnectionsResponse = {
	tmdb: { apiKey: '' },
	qbittorrent: { url: '', apiKey: '', category: 'magpie' },
	prowlarr: { url: '', apiKey: '' },
	jellyfin: { url: '', apiKey: '' },
	openSubtitles: { username: '', password: '' },
	health: []
};
const PASSWORD = 'twelve chars plus';
const TMDB_KEY = 'tmdb-key';

const sdk = vi.hoisted(() => ({
	setupAdmin: vi.fn(),
	getConnections: vi.fn(),
	updateConnections: vi.fn(),
	testConnection: vi.fn()
}));

vi.mock('$lib/sdk', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/sdk')>()),
	...sdk
}));

async function wizardPastAdmin(): Promise<SetupWizard> {
	const wizard = new SetupWizard();
	wizard.admin.username = 'admin';
	wizard.admin.password = wizard.admin.confirmation = PASSWORD;
	await wizard.createAdmin();
	await vi.waitFor(() => expect(sdk.getConnections).toHaveBeenCalled());
	return wizard;
}

const indexOf = (step: WizardStep) => WIZARD_STEPS.findIndex((spec) => spec.step === step);

describe('SetupWizard', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		sdk.setupAdmin.mockResolvedValue(ADMIN);
		sdk.getConnections.mockResolvedValue(EMPTY_CONNECTIONS);
		sdk.updateConnections.mockResolvedValue({ ...EMPTY_CONNECTIONS, tmdb: { apiKey: '••••••••' } });
	});

	it('opens only the admin step until the admin exists', () => {
		const wizard = new SetupWizard();
		expect(wizard.current.step).toBe(WizardStep.LOCAL_ADMIN);
		expect(wizard.canVisit(indexOf(WizardStep.TMDB))).toBe(false);
	});

	it('needs a 12-character password that matches its confirmation', () => {
		const wizard = new SetupWizard();
		wizard.admin.username = 'admin';
		wizard.admin.password = wizard.admin.confirmation = 'short';
		expect(wizard.admin.canSubmit).toBe(false);
		wizard.admin.password = PASSWORD;
		expect(wizard.admin.canSubmit).toBe(false);
		wizard.admin.confirmation = PASSWORD;
		expect(wizard.admin.canSubmit).toBe(true);
	});

	it('closes the admin step for good once the admin is created', async () => {
		const wizard = await wizardPastAdmin();
		expect(wizard.outcomes[WizardStep.LOCAL_ADMIN]).toBe(StepOutcome.DONE);
		expect(wizard.current.step).toBe(WizardStep.TMDB);
		expect(wizard.canVisit(indexOf(WizardStep.LOCAL_ADMIN))).toBe(false);
		expect(wizard.canVisit(indexOf(WizardStep.ARR_IMPORT))).toBe(true);
	});

	it('saves unsaved fields when a connection step is completed', async () => {
		const wizard = await wizardPastAdmin();
		wizard.connections.forms[Service.TMDB].values.apiKey = TMDB_KEY;
		await wizard.complete();

		expect(sdk.updateConnections).toHaveBeenCalledWith({ tmdb: { apiKey: TMDB_KEY } });
		expect(wizard.outcomes[WizardStep.TMDB]).toBe(StepOutcome.DONE);
		expect(wizard.current.step).toBe(WizardStep.QBITTORRENT);
	});

	it('marks skipped steps and keeps a done step done when skipped later', async () => {
		const wizard = await wizardPastAdmin();
		wizard.skip();
		expect(wizard.outcomes[WizardStep.TMDB]).toBe(StepOutcome.SKIPPED);

		wizard.visit(indexOf(WizardStep.TMDB));
		await wizard.complete();
		wizard.visit(indexOf(WizardStep.TMDB));
		wizard.skip();
		expect(wizard.outcomes[WizardStep.TMDB]).toBe(StepOutcome.DONE);
	});

	it('finishes after the last step', async () => {
		const wizard = await wizardPastAdmin();
		wizard.visit(WIZARD_STEPS.length - 1);
		expect(wizard.finished).toBe(false);
		wizard.skip();
		expect(wizard.finished).toBe(true);
	});
});
