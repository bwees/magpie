import * as client from './client';

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '';

client.defaults.baseUrl = apiBaseUrl;

export * from './client';
export { apiBaseUrl };

export function apiErrorMessage(
	error: unknown,
	fallback = 'Something went wrong. Please try again.'
): string {
	const data = (error as { data?: { detail?: string; title?: string } })?.data;
	return data?.detail ?? data?.title ?? fallback;
}
