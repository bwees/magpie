import { toast } from 'svelte-sonner';
import { HttpError } from '@oazapfts/runtime';
import {
	apiErrorMessage,
	getTitle,
	removeTitle,
	updateEpisode,
	updateSeason,
	updateTitle,
	type EpisodeResponse,
	type SeasonResponse,
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

export enum MediaKind {
	MOVIE = 'MOVIE',
	SERIES = 'SERIES'
}

/** Switches on the Title itself. */
enum TitleFlag {
	MONITORED = 'monitored',
	MONITOR_FUTURE_SEASONS = 'monitorFutureSeasons'
}

/** Catalog events that change what the title page shows. */
export enum TitleChangeEvent {
	MONITORING_CHANGED = 'catalog.MonitoringChanged',
	PROFILE_CHANGED = 'catalog.ProfileChanged',
	EPISODES_ANNOUNCED = 'catalog.EpisodesAnnounced',
	EPISODE_GROUP_CHANGED = 'catalog.EpisodeGroupChanged',
	LANGUAGES_CHANGED = 'catalog.LanguagesChanged',
	WANTED_RELEASED = 'catalog.WantedReleased',
	TITLE_REMOVED = 'catalog.TitleRemoved'
}

const TITLE_CHANGE_EVENTS = Object.values(TitleChangeEvent);

interface TitleEvent {
	title?: string;
}

export interface TitleApi {
	get(id: string): Promise<TitleResponse>;
	update(id: string, change: UpdateTitleRequest): Promise<TitleResponse>;
	updateSeason(id: string, season: number, monitored: boolean): Promise<TitleResponse>;
	updateEpisode(id: string, episodeId: string, monitored: boolean): Promise<TitleResponse>;
	remove(id: string, deleteFiles: boolean): Promise<unknown>;
	tint(posterUrl: string): Promise<string | undefined>;
}

export const CATALOG_TITLE_API: TitleApi = {
	get: (id) => getTitle(id),
	update: (id, change) => updateTitle(id, change),
	updateSeason: (id, season, monitored) => updateSeason(id, String(season), { monitored }),
	updateEpisode: (id, episodeId, monitored) => updateEpisode(id, episodeId, { monitored }),
	remove: (id, deleteFiles) => removeTitle(id, { deleteFiles }),
	tint: posterTint
};

/** Every episode's flag, to put the switches back when the server refuses a change. */
function monitoringOf(seasons: SeasonResponse[]): Map<Monitorable, boolean> {
	const items: Monitorable[] = seasons.flatMap((season) => [season, ...season.episodes]);
	return new Map(items.map((item) => [item, item.monitored]));
}

type Monitorable = SeasonResponse | EpisodeResponse;

/** One Title's page: loading, the optimistic Monitored switches and removal. */
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

	get isSeries(): boolean {
		return this.title?.kind === MediaKind.SERIES;
	}

	/** Shows a Title the server just returned for a change made elsewhere on the page. */
	replace(title: TitleResponse): void {
		this.title = title;
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

	setMonitored(monitored: boolean): Promise<void> {
		return this.#setFlag(TitleFlag.MONITORED, monitored);
	}

	/** Whether seasons TMDB announces later start monitored. */
	setMonitorFutureSeasons(monitored: boolean): Promise<void> {
		return this.#setFlag(TitleFlag.MONITOR_FUTURE_SEASONS, monitored);
	}

	async #setFlag(flag: TitleFlag, value: boolean): Promise<void> {
		const title = this.title;
		if (!title) {
			return;
		}
		title[flag] = value;
		try {
			this.title = await this.#api.update(this.id, { [flag]: value });
		} catch (error) {
			title[flag] = !value;
			toast.error(apiErrorMessage(error, 'Could not change monitoring.'));
		}
	}

	/** Monitors or unmonitors a season and all its episodes at once; the server confirms. */
	setSeasonMonitored(number: number, monitored: boolean): Promise<void> {
		const season = this.title?.seasons.find((s) => s.number === number);
		if (!season) return Promise.resolve();
		return this.#changeMonitoring([season, ...season.episodes], monitored, () =>
			this.#api.updateSeason(this.id, number, monitored)
		);
	}

	setEpisodeMonitored(episodeId: string, monitored: boolean): Promise<void> {
		const episode = this.title?.seasons
			.flatMap((season) => season.episodes)
			.find((e) => e.id === episodeId);
		if (!episode) return Promise.resolve();
		return this.#changeMonitoring([episode], monitored, () =>
			this.#api.updateEpisode(this.id, episodeId, monitored)
		);
	}

	async #changeMonitoring(
		items: Monitorable[],
		monitored: boolean,
		send: () => Promise<TitleResponse>
	): Promise<void> {
		const before = monitoringOf(this.title?.seasons ?? []);
		items.forEach((item) => (item.monitored = monitored));
		try {
			this.title = await send();
		} catch (error) {
			before.forEach((flag, item) => (item.monitored = flag));
			toast.error(apiErrorMessage(error, 'Could not change monitoring.'));
		}
	}

	async setProfile(profileId: string): Promise<void> {
		const title = this.title;
		if (!title || title.profileId === profileId) {
			return;
		}
		const previous = title.profileId;
		title.profileId = profileId;
		try {
			this.title = await this.#api.update(this.id, { profileId });
		} catch (error) {
			title.profileId = previous;
			toast.error(apiErrorMessage(error, 'Could not change the quality profile.'));
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
