/** How long an API request may take. Matches the server's write timeout, past which no answer arrives. */
export const REQUEST_TIMEOUT_MS = 30_000;

const MS_PER_SECOND = 1000;

export class RequestTimeoutError extends Error {
	constructor(timeoutMs: number) {
		super(
			`The server didn't answer within ${timeoutMs / MS_PER_SECOND} seconds. Please try again.`
		);
		this.name = 'RequestTimeoutError';
	}
}

const globalFetch: typeof fetch = (input, init) => fetch(input, init);

/**
 * Wraps fetch so a request gives up after timeoutMs with a RequestTimeoutError. A caller's own signal
 * still aborts it as before.
 */
export function withTimeout(timeoutMs: number, base: typeof fetch = globalFetch): typeof fetch {
	return async (input, init = {}) => {
		const timeout = new AbortController();
		const timer = setTimeout(() => timeout.abort(), timeoutMs);
		const signal = init.signal ? AbortSignal.any([init.signal, timeout.signal]) : timeout.signal;
		try {
			return await buffered(await base(input, { ...init, signal }));
		} catch (error) {
			if (timeout.signal.aborted) throw new RequestTimeoutError(timeoutMs);
			throw error;
		} finally {
			clearTimeout(timer);
		}
	};
}

// Statuses a Response can't be built with a body for, not even an empty one.
const NULL_BODY_STATUSES = new Set([101, 103, 204, 205, 304]);

// The generated client swallows a body that fails to read, so the body is read while the timeout runs.
async function buffered(response: Response): Promise<Response> {
	if (response.body === null || NULL_BODY_STATUSES.has(response.status)) return response;
	return new Response(await response.arrayBuffer(), {
		status: response.status,
		statusText: response.statusText,
		headers: response.headers
	});
}
