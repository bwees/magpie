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
	UNAUTHORIZED = 'urn:magpie:problem:unauthorized'
}

interface ProblemBody {
	type?: string;
	detail?: string;
	title?: string;
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
