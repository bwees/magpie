import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { DiscoverItemResponse, DiscoverResponse, TitleResponse } from '$lib/sdk';
import {
	LIBRARY_HIT_LIMIT,
	LoadState,
	SEARCH_DEBOUNCE_MS,
	TitleSearch,
	type SearchSource
} from './title-search.svelte';

function title(name: string): TitleResponse {
	return { id: name, name } as TitleResponse;
}

function item(name: string): DiscoverItemResponse {
	return { name, tmdbId: name.length, kind: 'MOVIE', inLibrary: false } as DiscoverItemResponse;
}

function discover(trending: boolean, ...names: string[]): DiscoverResponse {
	return { trending, items: names.map(item) };
}

class FakeSource implements SearchSource {
	libraryCalls: string[] = [];
	discoverCalls: string[] = [];
	libraryResult: TitleResponse[] = [];
	discoverResult: (query: string) => Promise<DiscoverResponse> = async (q) => discover(q === '', q);

	async library(query: string): Promise<TitleResponse[]> {
		this.libraryCalls.push(query);
		return this.libraryResult;
	}

	discover(query: string): Promise<DiscoverResponse> {
		this.discoverCalls.push(query);
		return this.discoverResult(query);
	}
}

describe('TitleSearch', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	it('shows trending when opened with an empty query', async () => {
		const source = new FakeSource();
		const search = new TitleSearch(source);

		search.show();
		await vi.runAllTimersAsync();

		expect(source.discoverCalls).toEqual(['']);
		expect(source.libraryCalls).toEqual([]);
		expect(search.trending).toBe(true);
		expect(search.tmdbState).toBe(LoadState.READY);
	});

	it('waits for typing to pause, then loads library hits and TMDB results', async () => {
		const source = new FakeSource();
		source.libraryResult = ['a', 'b', 'c', 'd', 'e'].map(title);
		const search = new TitleSearch(source);

		search.type('du');
		search.type('dune');
		await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);

		expect(source.discoverCalls).toEqual(['dune']);
		expect(search.libraryHits).toHaveLength(LIBRARY_HIT_LIMIT);
		expect(search.trending).toBe(false);
	});

	it('keeps library hits and reports TMDB as failed when TMDB is down', async () => {
		const source = new FakeSource();
		source.libraryResult = [title('Dune')];
		source.discoverResult = () => Promise.reject(new Error('503'));
		const search = new TitleSearch(source);

		search.type('dune');
		await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);

		expect(search.tmdbState).toBe(LoadState.FAILED);
		expect(search.libraryHits.map((t) => t.name)).toEqual(['Dune']);

		source.discoverResult = async (q) => discover(false, q);
		await search.retry();
		expect(search.tmdbState).toBe(LoadState.READY);
	});

	it('ignores an answer that arrives after a newer search started', async () => {
		const source = new FakeSource();
		let releaseSlow: (r: DiscoverResponse) => void = () => {};
		source.discoverResult = (q) =>
			q === 'slow'
				? new Promise((resolve) => (releaseSlow = resolve))
				: Promise.resolve(discover(false, q));
		const search = new TitleSearch(source);

		search.type('slow');
		await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
		search.type('fast');
		await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
		releaseSlow(discover(false, 'slow'));
		await vi.runAllTimersAsync();

		expect(search.tmdbItems.map((i) => i.name)).toEqual(['fast']);
	});

	it('clears everything on close', async () => {
		const search = new TitleSearch(new FakeSource());
		search.type('dune');
		await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);

		search.close();

		expect(search.open).toBe(false);
		expect(search.query).toBe('');
		expect(search.tmdbState).toBe(LoadState.IDLE);
	});
});
