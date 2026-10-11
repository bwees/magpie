import { ConnectionsState } from '$lib/connections/connections-state.svelte';
import { apiErrorMessage, setupAdmin, type MeResponse } from '$lib/sdk';
import { PASSWORD_HINTS, passwordProblem } from '$lib/session/password-form.svelte';
import { StepKind, StepOutcome, WIZARD_STEPS, WizardStep, type StepSpec } from './steps';

/** The first wizard step: create the local admin. */
export class AdminForm {
	username = $state('');
	password = $state('');
	confirmation = $state('');
	submitting = $state(false);
	error = $state('');

	get hint(): string | undefined {
		if (this.password === '') return undefined;
		const problem = passwordProblem(this.password, this.confirmation);
		return problem && PASSWORD_HINTS[problem];
	}

	get canSubmit(): boolean {
		return (
			this.username.trim() !== '' &&
			passwordProblem(this.password, this.confirmation) === undefined &&
			!this.submitting
		);
	}
}

function pendingOutcomes(): Record<WizardStep, StepOutcome> {
	return Object.fromEntries(WIZARD_STEPS.map((spec) => [spec.step, StepOutcome.PENDING])) as Record<
		WizardStep,
		StepOutcome
	>;
}

/**
 * The first-run wizard. Until the admin exists only the first step is open; after that the admin step is
 * closed for good and every other step can be done, skipped or revisited in any order.
 */
export class SetupWizard {
	index = $state(0);
	outcomes = $state(pendingOutcomes());
	finished = $state(false);
	readonly admin = new AdminForm();
	readonly connections = new ConnectionsState();

	get current(): StepSpec {
		return WIZARD_STEPS[this.index];
	}

	get isLast(): boolean {
		return this.index === WIZARD_STEPS.length - 1;
	}

	get adminCreated(): boolean {
		return this.outcomes[WizardStep.LOCAL_ADMIN] === StepOutcome.DONE;
	}

	canVisit(index: number): boolean {
		const spec = WIZARD_STEPS[index];
		if (!spec) return false;
		return spec.kind === StepKind.ACCOUNT ? !this.adminCreated : this.adminCreated;
	}

	visit(index: number): void {
		if (this.canVisit(index)) this.index = index;
	}

	/** Creates the local admin and signs them in. Returns the user for the session. */
	async createAdmin(): Promise<MeResponse | undefined> {
		const form = this.admin;
		form.submitting = true;
		form.error = '';
		try {
			const user = await setupAdmin({ username: form.username.trim(), password: form.password });
			form.password = form.confirmation = '';
			this.outcomes[WizardStep.LOCAL_ADMIN] = StepOutcome.DONE;
			this.#advance();
			this.connections.load();
			return user;
		} catch (error) {
			form.error = apiErrorMessage(error);
			return undefined;
		} finally {
			form.submitting = false;
		}
	}

	/** Saves the current step's unsaved edits, marks it done and moves on. */
	async complete(): Promise<void> {
		const service = this.current.service;
		if (service && this.connections.forms[service].dirty) {
			if (!(await this.connections.save(service))) return;
		}
		this.outcomes[this.current.step] = StepOutcome.DONE;
		this.#advance();
	}

	/** Moves on without doing the step. A step done earlier stays done. */
	skip(): void {
		if (this.outcomes[this.current.step] !== StepOutcome.DONE) {
			this.outcomes[this.current.step] = StepOutcome.SKIPPED;
		}
		this.#advance();
	}

	#advance(): void {
		if (this.isLast) {
			this.finished = true;
			return;
		}
		this.index += 1;
	}
}
