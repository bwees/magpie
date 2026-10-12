import { describe, expect, it, vi } from 'vitest';
import { HttpError } from '@oazapfts/runtime';
import { startSearch, type SearchResponse } from '$lib/sdk';

const CREATED = 201;
const NOT_FOUND = 404;
const JSON_CONTENT = 'application/json';
const PROBLEM_CONTENT = 'application/problem+json';
const TITLE_NOT_FOUND = 'urn:magpie:problem:title-not-found';
const TITLE = '01TITLE';

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
