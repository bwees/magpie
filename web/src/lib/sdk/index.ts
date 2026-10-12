import * as client from './client';

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '';

client.defaults.baseUrl = apiBaseUrl;

export * from './client';
export { apiBaseUrl };

/** Problem types the server sends in RFC 9457 `type`. Mirrors the Go `problem.Type` constants. */
export enum ProblemType {
	SETUP_REQUIRED = 'urn:magpie:problem:setup-required',
	SETUP_COMPLETE = 'urn:magpie:problem:setup-complete',
	WEAK_PASSWORD = 'urn:magpie:problem:weak-password',
	INVALID_CREDENTIALS = 'urn:magpie:problem:invalid-credentials',
	TOO_MANY_SIGN_INS = 'urn:magpie:problem:too-many-sign-ins',
	UNAUTHORIZED = 'urn:magpie:problem:unauthorized',
	JELLYFIN_UNAVAILABLE = 'urn:magpie:problem:jellyfin-unavailable',
	JELLYFIN_USER_DISABLED = 'urn:magpie:problem:jellyfin-user-disabled',
	USER_DISABLED = 'urn:magpie:problem:user-disabled',
	JELLYFIN_SIGN_IN_OFF = 'urn:magpie:problem:jellyfin-sign-in-off',
	QUICK_CONNECT_OFF = 'urn:magpie:problem:quick-connect-off',
	QUICK_CONNECT_EXPIRED = 'urn:magpie:problem:quick-connect-expired',
	LAST_ADMIN = 'urn:magpie:problem:last-admin',
	LOCAL_ADMIN_LOCKED = 'urn:magpie:problem:local-admin-locked',
	OWN_ACCOUNT = 'urn:magpie:problem:own-account'
}

interface ProblemBody {
	type?: string;
	detail?: string;
	title?: string;
	instance?: string;
}

function problemOf(error: unknown): ProblemBody | undefined {
	return (error as { data?: ProblemBody })?.data;
}

export function apiErrorMessage(
	error: unknown,
	fallback = 'Something went wrong. Please try again.'
): string {
	const data = problemOf(error);
	return data?.detail || data?.title || fallback;
}

export function apiErrorStatus(error: unknown): number | undefined {
	return (error as { status?: number })?.status;
}

export function apiProblemType(error: unknown): string | undefined {
	return problemOf(error)?.type;
}

/** The problem's `instance`: the API path of the thing it is about, when the server names one. */
export function apiProblemInstance(error: unknown): string | undefined {
	return problemOf(error)?.instance;
}
