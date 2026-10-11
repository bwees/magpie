import {
	apiErrorMessage,
	apiProblemType,
	getSignInMethods,
	pollQuickConnect,
	ProblemType,
	signInJellyfin,
	signInLocal,
	startQuickConnect,
	type MeResponse,
	type SignInMethodsResponse
} from '$lib/sdk';

export enum SignInMode {
	JELLYFIN = 'JELLYFIN',
	QUICK_CONNECT = 'QUICK_CONNECT',
	LOCAL = 'LOCAL'
}

export enum SignInErrorKind {
	INVALID_CREDENTIALS = 'INVALID_CREDENTIALS',
	JELLYFIN_UNAVAILABLE = 'JELLYFIN_UNAVAILABLE',
	DISABLED = 'DISABLED',
	OTHER = 'OTHER'
}

export interface SignInError {
	kind: SignInErrorKind;
	message: string;
}

/** Mirrors the Go `identity.QuickConnectStatus` values. */
export enum QuickConnectStatus {
	PENDING = 'PENDING',
	APPROVED = 'APPROVED'
}

export enum QuickConnectStep {
	STARTING = 'STARTING',
	WAITING = 'WAITING',
	EXPIRED = 'EXPIRED',
	FAILED = 'FAILED'
}

export const QUICK_CONNECT_POLL_MS = 2000;

const LOCAL_ONLY: SignInMethodsResponse = {
	jellyfin: false,
	quickConnect: false,
	jellyfinReachable: false
};

const SIGN_IN_ERRORS: Partial<Record<string, SignInError>> = {
	[ProblemType.INVALID_CREDENTIALS]: {
		kind: SignInErrorKind.INVALID_CREDENTIALS,
		message: 'Wrong username or password.'
	},
	[ProblemType.JELLYFIN_UNAVAILABLE]: {
		kind: SignInErrorKind.JELLYFIN_UNAVAILABLE,
		message: 'Jellyfin is not reachable right now.'
	},
	[ProblemType.JELLYFIN_USER_DISABLED]: {
		kind: SignInErrorKind.DISABLED,
		message: 'This Jellyfin account is disabled.'
	},
	[ProblemType.USER_DISABLED]: {
		kind: SignInErrorKind.DISABLED,
		message: 'This account is disabled in Magpie. Ask an admin to turn it back on.'
	}
};

export function signInErrorFor(error: unknown): SignInError {
	const known = SIGN_IN_ERRORS[apiProblemType(error) ?? ''];
	return known ?? { kind: SignInErrorKind.OTHER, message: apiErrorMessage(error) };
}

/** The sign-in page: Jellyfin password, Quick Connect, and the local admin form. */
export class SignInPage {
	methods = $state<SignInMethodsResponse>();
	mode = $state(SignInMode.JELLYFIN);
	username = $state('');
	password = $state('');
	submitting = $state(false);
	error = $state<SignInError>();
	quickConnectCode = $state('');
	quickConnectStep = $state<QuickConnectStep>();

	readonly #onSignedIn: (user: MeResponse) => void;
	#pollTimer: ReturnType<typeof setTimeout> | undefined;

	constructor(onSignedIn: (user: MeResponse) => void) {
		this.#onSignedIn = onSignedIn;
	}

	get canSubmit(): boolean {
		return this.username.trim() !== '' && this.password !== '' && !this.submitting;
	}

	get offersJellyfin(): boolean {
		return this.methods?.jellyfin ?? false;
	}

	async load(): Promise<void> {
		try {
			this.methods = await getSignInMethods();
		} catch {
			this.methods = LOCAL_ONLY;
		}
		this.mode = this.offersJellyfin ? SignInMode.JELLYFIN : SignInMode.LOCAL;
	}

	useLocalAdmin(): void {
		this.#switchTo(SignInMode.LOCAL);
	}

	useJellyfin(): void {
		this.#switchTo(SignInMode.JELLYFIN);
	}

	async submit(): Promise<void> {
		this.submitting = true;
		this.error = undefined;
		const credentials = { username: this.username.trim(), password: this.password };
		try {
			const user =
				this.mode === SignInMode.LOCAL
					? await signInLocal(credentials)
					: await signInJellyfin(credentials);
			this.#onSignedIn(user);
		} catch (error) {
			this.error = signInErrorFor(error);
			this.password = '';
		} finally {
			this.submitting = false;
		}
	}

	async startQuickConnect(): Promise<void> {
		this.#switchTo(SignInMode.QUICK_CONNECT);
		this.quickConnectStep = QuickConnectStep.STARTING;
		try {
			const started = await startQuickConnect();
			this.quickConnectCode = started.code;
			this.quickConnectStep = QuickConnectStep.WAITING;
			this.#schedulePoll(started.handle);
		} catch (error) {
			this.#quickConnectFailed(error);
		}
	}

	/** Stops polling; the page calls it when it goes away. */
	stop(): void {
		clearTimeout(this.#pollTimer);
		this.#pollTimer = undefined;
	}

	#switchTo(mode: SignInMode): void {
		this.stop();
		this.mode = mode;
		this.error = undefined;
		this.username = '';
		this.password = '';
		this.quickConnectCode = '';
		this.quickConnectStep = undefined;
	}

	#schedulePoll(handle: string): void {
		this.#pollTimer = setTimeout(() => this.#poll(handle), QUICK_CONNECT_POLL_MS);
	}

	async #poll(handle: string): Promise<void> {
		try {
			const result = await pollQuickConnect(handle);
			if (this.mode !== SignInMode.QUICK_CONNECT) return;
			if (result.status === QuickConnectStatus.APPROVED && result.user) {
				this.stop();
				this.#onSignedIn(result.user);
				return;
			}
			this.#schedulePoll(handle);
		} catch (error) {
			this.#quickConnectFailed(error);
		}
	}

	#quickConnectFailed(error: unknown): void {
		this.stop();
		if (apiProblemType(error) === ProblemType.QUICK_CONNECT_EXPIRED) {
			this.quickConnectStep = QuickConnectStep.EXPIRED;
			return;
		}
		this.quickConnectStep = QuickConnectStep.FAILED;
		this.error = signInErrorFor(error);
	}
}
