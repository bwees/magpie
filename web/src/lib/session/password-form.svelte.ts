import { toast } from 'svelte-sonner';
import { apiErrorMessage, changeLocalAdminPassword } from '$lib/sdk';

/** Mirrors identity.MinPasswordLength. */
export const MIN_PASSWORD_LENGTH = 12;

export enum PasswordProblem {
	TOO_SHORT = 'TOO_SHORT',
	MISMATCH = 'MISMATCH'
}

/** Checks a new password and its confirmation before it is sent. */
export function passwordProblem(
	password: string,
	confirmation: string
): PasswordProblem | undefined {
	if ([...password].length < MIN_PASSWORD_LENGTH) return PasswordProblem.TOO_SHORT;
	if (password !== confirmation) return PasswordProblem.MISMATCH;
	return undefined;
}

export const PASSWORD_HINTS: Record<PasswordProblem, string> = {
	[PasswordProblem.TOO_SHORT]: `Use at least ${MIN_PASSWORD_LENGTH} characters.`,
	[PasswordProblem.MISMATCH]: 'The passwords do not match.'
};

/** Settings → General: change the local admin's password. */
export class PasswordForm {
	current = $state('');
	next = $state('');
	confirmation = $state('');
	submitting = $state(false);
	error = $state('');

	get problem(): PasswordProblem | undefined {
		return passwordProblem(this.next, this.confirmation);
	}

	get canSubmit(): boolean {
		return this.current !== '' && this.problem === undefined && !this.submitting;
	}

	async submit(): Promise<void> {
		this.submitting = true;
		this.error = '';
		try {
			await changeLocalAdminPassword({ currentPassword: this.current, newPassword: this.next });
			this.current = this.next = this.confirmation = '';
			toast.success('Password changed', { description: 'Other sessions were signed out.' });
		} catch (error) {
			this.error = apiErrorMessage(error);
		} finally {
			this.submitting = false;
		}
	}
}
