import { toast } from 'svelte-sonner';
import { SvelteSet } from 'svelte/reactivity';
import { apiErrorMessage, startSearch } from '$lib/sdk';

/** What a Search button searches: a whole title, or some of its episodes. */
export interface SearchTarget {
	titleId: string;
	episodeIds?: string[];
	/** Names the target in the toast, e.g. "Severance" or "S01E03". */
	label: string;
}

export interface AutomaticSearchApi {
	start: (target: SearchTarget) => Promise<unknown>;
}

export interface Notifier {
	success: (message: string, options?: { description?: string }) => void;
	error: (message: string) => void;
}

const sdkApi: AutomaticSearchApi = {
	start: (target) =>
		startSearch({ title: target.titleId, episodes: target.episodeIds, interactive: false })
};

export const PROGRESS_HINT = 'Follow it in the stage bar and the timeline.';

function targetKey(target: SearchTarget): string {
	return [target.titleId, ...(target.episodeIds ?? [])].join(':');
}

/**
 * The title page's Search button and the episode-row search icon. Each starts a search that grabs the best
 * release on its own; the button stays busy until the server has accepted it.
 */
export class AutomaticSearch {
	readonly #starting = new SvelteSet<string>();
	readonly #api: AutomaticSearchApi;
	readonly #notify: Notifier;

	constructor(api: AutomaticSearchApi = sdkApi, notify: Notifier = toast) {
		this.#api = api;
		this.#notify = notify;
	}

	isStarting(target: SearchTarget): boolean {
		return this.#starting.has(targetKey(target));
	}

	async search(target: SearchTarget): Promise<void> {
		const key = targetKey(target);
		if (this.#starting.has(key)) {
			return;
		}
		this.#starting.add(key);
		try {
			await this.#api.start(target);
			this.#notify.success(`Searching for ${target.label}`, { description: PROGRESS_HINT });
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, `Could not search for ${target.label}.`));
		} finally {
			this.#starting.delete(key);
		}
	}
}
