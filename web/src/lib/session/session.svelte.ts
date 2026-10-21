import { createContext } from 'svelte';
import {
	apiErrorMessage,
	apiErrorStatus,
	apiProblemType,
	getMe,
	ProblemType,
	signOut
} from '$lib/sdk';
import type { MeResponse } from '$lib/sdk';
import { mayOpen, NavRole } from '$lib/navigation/nav';

export enum SessionStatus {
	LOADING = 'LOADING',
	SIGNED_IN = 'SIGNED_IN',
	SIGNED_OUT = 'SIGNED_OUT',
	SETUP_REQUIRED = 'SETUP_REQUIRED',
	ERROR = 'ERROR'
}

/** Mirrors the Go `api.Role` values. */
export enum Role {
	ADMIN = 'ADMIN',
	REQUESTER = 'REQUESTER'
}

/** Pages shown without the app shell. */
export enum AuthRoute {
	SIGN_IN = '/sign-in',
	SETUP = '/setup'
}

const HOME = '/';
const HTTP_UNAUTHORIZED = 401;

export function isAuthRoute(pathname: string): boolean {
	return Object.values<string>(AuthRoute).includes(pathname);
}

/**
 * Where a visitor on pathname must go, or undefined to stay. The wizard is reachable only while no admin
 * exists, or right after the admin was created in this tab; once left, it can't be reopened.
 * Non-admins are sent home from pages their role may not open.
 */
export function redirectFor(
	status: SessionStatus,
	pathname: string,
	wizardActive: boolean,
	role: NavRole
): string | undefined {
	switch (status) {
		case SessionStatus.SETUP_REQUIRED:
			return pathname === AuthRoute.SETUP ? undefined : AuthRoute.SETUP;
		case SessionStatus.SIGNED_OUT:
			return pathname === AuthRoute.SIGN_IN ? undefined : AuthRoute.SIGN_IN;
		case SessionStatus.SIGNED_IN:
			if (pathname === AuthRoute.SIGN_IN) return HOME;
			if (pathname === AuthRoute.SETUP && !wizardActive) return HOME;
			if (!mayOpen(pathname, role)) return HOME;
			return undefined;
		default:
			return undefined;
	}
}

export class SessionState {
	status = $state(SessionStatus.LOADING);
	user = $state<MeResponse | undefined>();
	error = $state('');
	wizardActive = $state(false);

	async load(): Promise<void> {
		this.status = SessionStatus.LOADING;
		try {
			this.signedIn(await getMe());
		} catch (error) {
			this.#failed(error);
		}
	}

	/** Called after setup or sign-in returned the user and set the session cookie. */
	signedIn(user: MeResponse, { startWizard = false } = {}): void {
		this.user = user;
		this.wizardActive = startWizard;
		this.status = SessionStatus.SIGNED_IN;
	}

	finishWizard(): void {
		this.wizardActive = false;
	}

	async signOut(): Promise<void> {
		try {
			await signOut();
		} finally {
			this.user = undefined;
			this.wizardActive = false;
			this.status = SessionStatus.SIGNED_OUT;
		}
	}

	get isAdmin(): boolean {
		return this.user?.role === Role.ADMIN;
	}

	/** Admins always may; requesters need the "report issues" toggle. */
	get canReportIssues(): boolean {
		return this.isAdmin || (this.user?.permissions.reportIssues ?? false);
	}

	/** Admins always may; requesters need the "manage files" toggle, and then only for titles they may see. */
	get canManageFiles(): boolean {
		return this.isAdmin || (this.user?.permissions.manageFiles ?? false);
	}

	get navRole(): NavRole {
		if (this.isAdmin) return NavRole.ADMIN;
		return this.canManageFiles ? NavRole.FILE_MANAGER : NavRole.REQUESTER;
	}

	redirectFor(pathname: string): string | undefined {
		return redirectFor(this.status, pathname, this.wizardActive, this.navRole);
	}

	#failed(error: unknown): void {
		this.user = undefined;
		if (apiProblemType(error) === ProblemType.SETUP_REQUIRED) {
			this.status = SessionStatus.SETUP_REQUIRED;
		} else if (apiErrorStatus(error) === HTTP_UNAUTHORIZED) {
			this.status = SessionStatus.SIGNED_OUT;
		} else {
			this.error = apiErrorMessage(error, 'Magpie is not responding.');
			this.status = SessionStatus.ERROR;
		}
	}
}

export const [getSession, setSession] = createContext<SessionState>();
