import { discoverTitles, type DiscoverItemResponse } from '$lib/sdk';
import { DataBlock } from '$lib/data-block.svelte';

/** TMDB's trending titles. The SDK in the app, a fake in tests. */
export type TrendingSource = () => Promise<DiscoverItemResponse[]>;

export const SDK_TRENDING: TrendingSource = async () => (await discoverTitles({ q: '' })).items;

/** The requester Home's trending row. Request buttons go through the shell's search requests. */
export class Trending {
	readonly block: DataBlock<DiscoverItemResponse[]>;

	constructor(source: TrendingSource = SDK_TRENDING) {
		this.block = new DataBlock({
			fetch: source,
			initial: [],
			isEmpty: (items) => items.length === 0,
			failure: 'TMDB is unavailable.'
		});
	}

	get items(): DiscoverItemResponse[] {
		return this.block.data;
	}

	load(): Promise<void> {
		return this.block.load();
	}
}
