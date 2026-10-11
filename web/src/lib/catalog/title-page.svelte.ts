import { toast } from 'svelte-sonner';
import { HttpError } from '@oazapfts/runtime';
import {
	apiErrorMessage,
	getTitle,
	removeTitle,
	updateTitle,
	type TitleResponse,
	type UpdateTitleRequest
} from '$lib/sdk';
import type { LiveConnection } from '$lib/live/live-connection';
import type { LiveMessage } from '$lib/live/protocol';
import { ImageWidth, sizedImage } from './images';
import { posterTint } from './poster-tint';

export enum PageStatus {
	LOADING = 'LOADING',
	READY = 'READY',
	NOT_FOUND = 'NOT_FOUND',
	FAILED = 'FAILED'
}

const NOT_FOUND_STATUS = 404;

/** Catalog events that change what the title page shows. */
export enum TitleChangeEvent {
	MONITORING_CHANGED = 'catalog.MonitoringChanged',
	PROFILE_CHANGED = 'catalog.ProfileChanged',
	TITLE_REMOVED = 'catalog.TitleRemoved'
}

const TITLE_CHANGE_EVENTS = Object.values(TitleChangeEvent);

interface TitleEvent {
	title?: string;
}

export interface TitleApi {
	get(id: string): Promise<TitleResponse>;
	update(id: string, change: UpdateTitleRequest): Promise<TitleResponse>;
	remove(id: string, deleteFiles: boolean): Promise<unknown>;
	tint(posterUrl: string): Promise<string | undefined>;
}

export const CATALOG_TITLE_API: TitleApi = {
	get: (id) => getTitle(id),
	update: (id, change) => updateTitle(id, change),
	remove: (id, deleteFiles) => removeTitle(id, { deleteFiles }),
	tint: posterTint
};

/** One Title's page: loading, the optimistic Monitored switch and removal. */
export class TitlePage {
	status = $state(PageStatus.LOADING);
	title = $state<TitleResponse | undefined>();
	error = $state('');
	tint = $state<string | undefined>();
	confirmingRemove = $state(false);
	deleteFiles = $state(false);
	removing = $state(false);

	readonly id: string;
	readonly #api: TitleApi;

	constructor(id: string, api: TitleApi = CATALOG_TITLE_API) {
		this.id = id;
		this.#api = api;
	}

	get posterUrl(): string | undefined {
		return sizedImage(this.title?.images.poster, ImageWidth.POSTER);
	}

	async load(): Promise<void> {
		if (!this.title) {
			this.status = PageStatus.LOADING;
		}
		try {
			this.title = await this.#api.get(this.id);
			this.status = PageStatus.READY;
		} catch (error) {
			this.status =
				error instanceof HttpError && error.status === NOT_FOUND_STATUS
					? PageStatus.NOT_FOUND
					: PageStatus.FAILED;
			this.error = apiErrorMessage(error);
			return;
		}
		const poster = this.posterUrl;
		if (poster && !this.tint) {
			this.tint = await this.#api.tint(poster);
		}
	}

	async setMonitored(monitored: boolean): Promise<void> {
		const title = this.title;
		if (!title) {
			return;
		}
		title.monitored = monitored;
		try {
			this.title = await this.#api.update(this.id, { monitored });
		} catch (error) {
			title.monitored = !monitored;
			toast.error(apiErrorMessage(error, 'Could not change monitoring.'));
		}
	}

	/** Refetches when Catalog changes this Title or the live socket asks for a refetch. */
	watch(live: LiveConnection): () => void {
		const refetchIfMine = (message: LiveMessage) => {
			if ((message.data as TitleEvent | undefined)?.title === this.id) {
				void this.load();
			}
		};
		const stops = TITLE_CHANGE_EVENTS.map((type) => live.on(type, refetchIfMine));
		stops.push(live.onReset(() => void this.load()));
		return () => stops.forEach((stop) => stop());
	}

	askToRemove(): void {
		this.deleteFiles = false;
		this.confirmingRemove = true;
	}

	/** Removes the Title; true when it is gone and the page should leave. */
	async remove(): Promise<boolean> {
		this.removing = true;
		try {
			await this.#api.remove(this.id, this.deleteFiles);
			toast.success(`${this.title?.name ?? 'Title'} removed`);
			this.confirmingRemove = false;
			return true;
		} catch (error) {
			toast.error(apiErrorMessage(error, 'Could not remove the title.'));
			return false;
		} finally {
			this.removing = false;
		}
	}
}
