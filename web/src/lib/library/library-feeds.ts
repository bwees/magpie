import {
	getCoverage,
	listMediaFiles,
	type MediaFileResponse,
	type TitleCoverageResponse
} from '$lib/sdk';
import { chunked } from '$lib/catalog/title-directory.svelte';

/** Library's batch reads behind the Movies and TV pages. The SDK in the app, a fake in tests. */
export interface LibraryFeeds {
	/** The files of each title, grouped by title. */
	files(titles: string[]): Promise<MediaFileResponse[]>;
	/** Episodes with a file, episodes Jellyfin confirmed, and size, per title. */
	coverage(titles: string[]): Promise<TitleCoverageResponse[]>;
}

async function batched<T>(titles: string[], read: (chunk: string[]) => Promise<T[]>): Promise<T[]> {
	if (titles.length === 0) return [];
	return (await Promise.all(chunked(titles).map(read))).flat();
}

export const SDK_LIBRARY_FEEDS: LibraryFeeds = {
	files: (titles) => batched(titles, (chunk) => listMediaFiles({ titles: chunk })),
	coverage: (titles) => batched(titles, (chunk) => getCoverage({ titles: chunk }))
};

/** Groups items by the title they belong to. */
export function byTitle<T>(items: T[], titleOf: (item: T) => string): Record<string, T[]> {
	const groups: Record<string, T[]> = {};
	for (const item of items) (groups[titleOf(item)] ??= []).push(item);
	return groups;
}
