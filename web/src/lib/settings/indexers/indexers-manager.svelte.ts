import {
	apiErrorMessage,
	getIndexerOptions,
	listIndexers,
	setIndexerOptions,
	syncIndexers,
	updateIndexer,
	type IndexerListResponse,
	type IndexerResponse
} from '$lib/sdk';
import {
	indexerDraft,
	indexerDraftError,
	optionsDraft,
	optionsDraftError,
	optionsRequest,
	updateRequest,
	type IndexerDraft,
	type OptionsDraft
} from './indexer-settings';

export enum LoadState {
	LOADING = 'LOADING',
	READY = 'READY',
	FAILED = 'FAILED'
}

/** One table row: the indexer as saved plus the inputs being edited. */
export class IndexerRow {
	indexer: IndexerResponse;
	draft: IndexerDraft;
	error = $state<string>();
	saving = $state(false);

	constructor(indexer: IndexerResponse) {
		this.indexer = $state(indexer);
		this.draft = $state(indexerDraft(indexer));
	}

	reset(indexer: IndexerResponse) {
		this.indexer = indexer;
		this.draft = indexerDraft(indexer);
		this.error = undefined;
	}
}

/** State for Settings → Indexers: the list from Prowlarr, sync, per-indexer edits and shared options. */
export class IndexersManager {
	loadState = $state(LoadState.LOADING);
	loadError = $state<string>();
	rows = $state<IndexerRow[]>([]);
	prowlarrUrl = $state('');
	lastSyncedAt = $state<string>();
	syncError = $state<string>();
	syncing = $state(false);

	options = $state<OptionsDraft>();
	optionsError = $state<string>();
	savingOptions = $state(false);

	readonly #onSynced: (count: number) => void;

	constructor(onSynced: (count: number) => void = () => {}) {
		this.#onSynced = onSynced;
	}

	/** Prowlarr answered and has no indexers, as opposed to Prowlarr being unreachable. */
	get isEmpty(): boolean {
		return this.loadState === LoadState.READY && this.rows.length === 0 && !this.syncError;
	}

	get showsTable(): boolean {
		return this.loadState === LoadState.LOADING || this.rows.length > 0;
	}

	async load() {
		this.loadState = LoadState.LOADING;
		try {
			const [list, options] = await Promise.all([listIndexers(), getIndexerOptions()]);
			this.#apply(list);
			this.options = optionsDraft(options);
			this.loadError = undefined;
			this.loadState = LoadState.READY;
		} catch (error) {
			this.loadError = apiErrorMessage(error, 'Could not load indexers.');
			this.loadState = LoadState.FAILED;
		}
	}

	async syncNow() {
		this.syncing = true;
		try {
			const list = await syncIndexers();
			this.#apply(list);
			this.#onSynced(list.indexers.length);
		} catch (error) {
			this.syncError = apiErrorMessage(error, 'Prowlarr sync failed.');
		} finally {
			this.syncing = false;
		}
	}

	async save(row: IndexerRow) {
		row.error = indexerDraftError(row.draft);
		if (row.error) {
			return;
		}
		row.saving = true;
		try {
			row.reset(await updateIndexer(row.indexer.id, updateRequest(row.draft)));
		} catch (error) {
			row.error = apiErrorMessage(error, 'Could not save.');
		} finally {
			row.saving = false;
		}
	}

	async saveOptions() {
		if (!this.options) {
			return;
		}
		this.optionsError = optionsDraftError(this.options);
		if (this.optionsError) {
			return;
		}
		this.savingOptions = true;
		try {
			this.options = optionsDraft(await setIndexerOptions(optionsRequest(this.options)));
		} catch (error) {
			this.optionsError = apiErrorMessage(error, 'Could not save.');
		} finally {
			this.savingOptions = false;
		}
	}

	#apply(list: IndexerListResponse) {
		this.rows = list.indexers.map((indexer) => new IndexerRow(indexer));
		this.prowlarrUrl = list.prowlarrUrl;
		this.lastSyncedAt = list.lastSyncedAt;
		this.syncError = list.syncError;
	}
}
