import { apiErrorMessage, signInLocal, type MeResponse } from '$lib/sdk';

/** The local admin sign-in form. */
export class SignInForm {
	username = $state('');
	password = $state('');
	submitting = $state(false);
	error = $state('');

	get canSubmit(): boolean {
		return this.username.trim() !== '' && this.password !== '' && !this.submitting;
	}

	/** Returns the signed-in user, or undefined with error set. */
	async submit(): Promise<MeResponse | undefined> {
		this.submitting = true;
		this.error = '';
		try {
			return await signInLocal({ username: this.username.trim(), password: this.password });
		} catch (error) {
			this.error = apiErrorMessage(error);
			this.password = '';
			return undefined;
		} finally {
			this.submitting = false;
		}
	}
}
