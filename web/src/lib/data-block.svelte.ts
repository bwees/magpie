import { apiErrorMessage } from '$lib/sdk';
import { LoadStatus } from './load-status';
import { coalesce } from './live/coalesce';

export interface DataBlockOptions<T> {
	fetch: () => Promise<T>;
	initial: T;
	isEmpty: (data: T) => boolean;
	/** The inline error when the server gave no detail. */
	failure: string;
}

/**
 * One data block's fetch: a skeleton until the first answer, then content or an empty state, or an
 * inline error with Retry. A failed refetch keeps what is shown; refetches during one run once more.
 */
export class DataBlock<T> {
	data: T = $state() as T;
	status = $state(LoadStatus.LOADING);
	error = $state('');

	readonly #options: DataBlockOptions<T>;
	readonly #refetch: () => Promise<void>;
	#loaded = false;

	constructor(options: DataBlockOptions<T>) {
		this.#options = options;
		this.data = options.initial;
		this.#refetch = coalesce(() => this.#run());
	}

	get empty(): boolean {
		return this.status === LoadStatus.READY && this.#options.isEmpty(this.data);
	}

	get ready(): boolean {
		return this.status === LoadStatus.READY;
	}

	load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		this.#loaded = false;
		return this.#refetch();
	}

	refresh(): Promise<void> {
		return this.#refetch();
	}

	async #run(): Promise<void> {
		try {
			this.data = await this.#options.fetch();
			this.error = '';
			this.#loaded = true;
			this.status = LoadStatus.READY;
		} catch (error) {
			this.error = apiErrorMessage(error, this.#options.failure);
			if (!this.#loaded) this.status = LoadStatus.ERROR;
		}
	}
}
