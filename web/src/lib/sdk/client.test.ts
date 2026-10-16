import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { HttpError } from '@oazapfts/runtime';
import {
	REQUEST_TIMEOUT_MS,
	RequestTimeoutError,
	apiErrorMessage,
	deleteSubtitle,
	startSearch,
	type SearchResponse
} from '$lib/sdk';

const CREATED = 201;
const NO_CONTENT = 204;
const NOT_FOUND = 404;
const JSON_CONTENT = 'application/json';
const PROBLEM_CONTENT = 'application/problem+json';
const TITLE_NOT_FOUND = 'urn:magpie:problem:title-not-found';
const TITLE = '01TITLE';
const SUBTITLE = '01SUBTITLE';

const SEARCH: SearchResponse = {
	searchId: '01SEARCH',
	title: TITLE,
	trigger: 'INTERACTIVE',
	startedAt: '2026-10-11T00:00:00Z',
	completed: false,
	cancelled: false,
	indexers: [],
	releases: [],
	profile: { id: '01PROFILE', name: '1080p', cutoff: '1080p', buckets: [], penalizedFormats: [] }
};

function answer(status: number, contentType: string, body: unknown) {
	return vi.fn(
		async () =>
			new Response(JSON.stringify(body), { status, headers: { 'Content-Type': contentType } })
	);
}

/** A fetch whose server never answers: it only settles when its signal aborts. */
function hanging() {
	return vi.fn(
		(_input: RequestInfo | URL, init?: RequestInit) =>
			new Promise<Response>((_, reject) => {
				init?.signal?.addEventListener('abort', () => reject(init.signal?.reason));
			})
	);
}

const START = { title: TITLE, interactive: true };

describe('request timeout', () => {
	beforeEach(() => vi.useFakeTimers());

	afterEach(() => {
		vi.useRealTimers();
		vi.unstubAllGlobals();
	});

	it('gives up on a request the server never answers, with a message saying so', async () => {
		vi.stubGlobal('fetch', hanging());

		const failure = startSearch(START).catch((error: unknown) => error);
		await vi.advanceTimersByTimeAsync(REQUEST_TIMEOUT_MS);

		const error = await failure;
		expect(error).toBeInstanceOf(RequestTimeoutError);
		expect(apiErrorMessage(error, 'Could not start the search.')).toBe(
			"The server didn't answer within 30 seconds. Please try again."
		);
	});

	it('keeps waiting until the timeout is up', async () => {
		vi.stubGlobal('fetch', hanging());
		let settled = false;

		const failure = startSearch(START).finally(() => (settled = true));
		failure.catch(() => undefined);
		await vi.advanceTimersByTimeAsync(REQUEST_TIMEOUT_MS - 1);

		expect(settled).toBe(false);
	});

	it("leaves the caller's own abort as it was", async () => {
		vi.stubGlobal('fetch', hanging());
		const caller = new AbortController();

		const failure = startSearch(START, {}, { signal: caller.signal }).catch(
			(error: unknown) => error
		);
		caller.abort();

		const error = await failure;
		expect(error).not.toBeInstanceOf(RequestTimeoutError);
		expect((error as DOMException).name).toBe('AbortError');
	});

	it('returns an answer that arrives in time', async () => {
		vi.stubGlobal('fetch', answer(CREATED, JSON_CONTENT, SEARCH));

		expect(await startSearch(START)).toEqual(SEARCH);
	});

	it('returns a 204 whose body the browser hands over as an empty stream', async () => {
		const noContent = new Response(null, { status: NO_CONTENT });
		Object.defineProperty(noContent, 'body', { value: new ReadableStream() });
		vi.stubGlobal(
			'fetch',
			vi.fn(async () => noContent)
		);

		await expect(deleteSubtitle(SUBTITLE)).resolves.not.toThrow();
	});
});

describe('generated client', () => {
	it('returns the parsed body of a 201 route', async () => {
		const fetch = answer(CREATED, JSON_CONTENT, SEARCH);

		const search = await startSearch({ title: TITLE, interactive: true }, {}, { fetch });

		expect(search).toEqual(SEARCH);
	});

	it('rejects with the parsed problem on an error', async () => {
		const problem = { type: TITLE_NOT_FOUND, status: NOT_FOUND, title: 'Not Found' };
		const fetch = answer(NOT_FOUND, PROBLEM_CONTENT, problem);

		const failure = await startSearch({ title: TITLE, interactive: true }, {}, { fetch }).catch(
			(error: unknown) => error
		);

		expect(failure).toBeInstanceOf(HttpError);
		expect((failure as HttpError).data).toEqual(problem);
	});
});
