import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	discoverTitles,
	ignoreScanReview,
	listScanReview,
	matchScanReview,
	type DiscoverItemResponse,
	type ScanCandidateResponse,
	type ScanReviewResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';

export interface ScanReviewApi {
	list: () => Promise<ScanReviewResponse[]>;
	match: (id: string, tmdbId: number) => Promise<ScanReviewResponse>;
	ignore: (id: string) => Promise<ScanReviewResponse>;
	search: (query: string) => Promise<DiscoverItemResponse[]>;
}

export interface Notifier {
	success: (message: string, options?: { description?: string }) => void;
	error: (message: string, options?: { description?: string }) => void;
}

/** A TMDB title a folder might be: a scored candidate from the scan, or a search result without a score. */
export type ReviewCandidate = Pick<
	ScanCandidateResponse,
	'tmdbId' | 'name' | 'year' | 'overview' | 'posterUrl'
> & { score?: number };

/** One row's TMDB search: what was typed and, once searched, what TMDB returned. */
export interface CandidateSearch {
	query: string;
	results?: ReviewCandidate[];
	searching: boolean;
}

const PERCENT = 100;

const sdkApi: ScanReviewApi = {
	list: () => listScanReview(),
	match: (id, tmdbId) => matchScanReview(id, { tmdbId }),
	ignore: (id) => ignoreScanReview(id),
	search: async (query) => (await discoverTitles({ q: query })).items
};

export function formatScore(score: number): string {
	return `${Math.round(score * PERCENT)}%`;
}

/** State behind Settings → Library's review queue: folders a scan couldn't match with certainty. */
export class ScanReviewManager {
	rows = $state<ScanReviewResponse[]>([]);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	readonly busy = new SvelteSet<string>();
	readonly rowErrors = new SvelteMap<string, string>();
	readonly searches = new SvelteMap<string, CandidateSearch>();

	readonly #api: ScanReviewApi;
	readonly #notify: Notifier;

	constructor(api: ScanReviewApi = sdkApi, notify: Notifier = toast) {
		this.#api = api;
		this.#notify = notify;
	}

	get count(): number {
		return this.rows.length;
	}

	async load(): Promise<void> {
		try {
			this.rows = await this.#api.list();
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			if (this.status !== LoadStatus.READY) this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load the folders to review.');
		}
	}

	/** The row's search results once it searched, else the scan's candidates. */
	candidates(row: ScanReviewResponse): ReviewCandidate[] {
		return this.searches.get(row.id)?.results ?? row.candidates;
	}

	query(row: ScanReviewResponse): string {
		return this.searches.get(row.id)?.query ?? row.parsedTitle;
	}

	setQuery(row: ScanReviewResponse, query: string): void {
		this.searches.set(row.id, { ...this.#search(row), query });
	}

	async search(row: ScanReviewResponse): Promise<void> {
		const query = this.query(row).trim();
		if (!query) return;
		this.searches.set(row.id, { ...this.#search(row), searching: true });
		try {
			const items = await this.#api.search(query);
			const results = items.filter((item) => item.kind === row.kind);
			this.searches.set(row.id, { query, results, searching: false });
			this.rowErrors.delete(row.id);
		} catch (error) {
			this.searches.set(row.id, { ...this.#search(row), searching: false });
			this.rowErrors.set(row.id, apiErrorMessage(error, 'Could not search TMDB.'));
		}
	}

	match(row: ScanReviewResponse, candidate: ReviewCandidate): Promise<void> {
		return this.#resolve(row, async () => {
			await this.#api.match(row.id, candidate.tmdbId);
			this.#notify.success(`Matched to ${candidate.name}`, { description: row.folderPath });
		});
	}

	ignore(row: ScanReviewResponse): Promise<void> {
		return this.#resolve(row, async () => {
			await this.#api.ignore(row.id);
			this.#notify.success('Folder ignored', { description: row.folderPath });
		});
	}

	#search(row: ScanReviewResponse): CandidateSearch {
		return this.searches.get(row.id) ?? { query: row.parsedTitle, searching: false };
	}

	async #resolve(row: ScanReviewResponse, action: () => Promise<void>): Promise<void> {
		this.busy.add(row.id);
		try {
			await action();
			this.rows = this.rows.filter((r) => r.id !== row.id);
			this.rowErrors.delete(row.id);
			this.searches.delete(row.id);
		} catch (error) {
			this.rowErrors.set(row.id, apiErrorMessage(error, 'Could not update the folder.'));
		} finally {
			this.busy.delete(row.id);
		}
	}
}
