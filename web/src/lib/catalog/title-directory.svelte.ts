import { SvelteMap } from 'svelte/reactivity';
import { listTitles, type TitleResponse } from '$lib/sdk';

/** The most titles one GET /api/catalog/titles?ids= may name. */
export const MAX_IDS_PER_LOOKUP = 500;

/** Reads Titles by ID. The SDK in the app, a fake in tests. */
export type TitlesById = (ids: string[]) => Promise<TitleResponse[]>;

export const SDK_TITLES_BY_ID: TitlesById = (ids) => listTitles({ ids });

/** The items without repeats, in first-seen order. */
export function unique<T>(items: T[]): T[] {
	return items.filter((item, index) => items.indexOf(item) === index);
}

/** Splits ids into lookups the server accepts. */
export function chunked<T>(items: T[], size = MAX_IDS_PER_LOOKUP): T[][] {
	const chunks: T[][] = [];
	for (let start = 0; start < items.length; start += size) {
		chunks.push(items.slice(start, start + size));
	}
	return chunks;
}

/**
 * Names and posters of Titles other contexts point at by ID, fetched once each. Blocks that list
 * Activity rows use it to show what the rows are about.
 */
export class TitleDirectory {
	readonly titles = new SvelteMap<string, TitleResponse>();
	readonly #byId: TitlesById;

	constructor(byId: TitlesById = SDK_TITLES_BY_ID) {
		this.#byId = byId;
	}

	get(id: string): TitleResponse | undefined {
		return this.titles.get(id);
	}

	/** Fetches the titles not known yet. A failure leaves them unknown; callers show the row without them. */
	async ensure(ids: string[]): Promise<void> {
		const missing = unique(ids).filter((id) => !this.titles.has(id));
		await Promise.all(
			chunked(missing).map(async (chunk) => {
				try {
					for (const title of await this.#byId(chunk)) this.titles.set(title.id, title);
				} catch {
					// Unknown titles show as "Unknown title" until the next refetch tries again.
				}
			})
		);
	}

	/** Forgets a title, so the next ensure refetches it, e.g. after it was renamed or removed. */
	forget(id: string): void {
		this.titles.delete(id);
	}
}
