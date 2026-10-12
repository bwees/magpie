import {
	getAttention,
	getPipeline,
	type AttentionItemResponse,
	type PipelineRowResponse
} from '$lib/sdk';
import { chunked } from '$lib/catalog/title-directory.svelte';

/** Activity's lists behind Home, the library pages and the nav badges. The SDK in the app, a fake in tests. */
export interface ActivityFeeds {
	attention(): Promise<AttentionItemResponse[]>;
	/** Rows searching, downloading, importing or transcoding. */
	active(): Promise<PipelineRowResponse[]>;
	/** The summary row of the most recently changed titles, newest first. */
	recent(): Promise<PipelineRowResponse[]>;
	/** The summary row of each title Activity knows. */
	stages(titles: string[]): Promise<PipelineRowResponse[]>;
}

export const SDK_FEEDS: ActivityFeeds = {
	attention: () => getAttention(),
	active: () => getPipeline({ active: true }),
	recent: () => getPipeline({ recent: true }),
	stages: async (titles) => {
		if (titles.length === 0) return [];
		const pages = await Promise.all(chunked(titles).map((chunk) => getPipeline({ titles: chunk })));
		return pages.flat();
	}
};
