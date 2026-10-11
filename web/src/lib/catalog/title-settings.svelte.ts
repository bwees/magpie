import { toast } from 'svelte-sonner';
import { LoadStatus } from '$lib/load-status';
import {
	apiErrorMessage,
	getWantedLanguages,
	listEpisodeGroups,
	selectEpisodeGroup,
	updateTitleLanguages,
	type EpisodeGroupResponse,
	type TitleResponse,
	type WantedLanguageEntry,
	type WantedLanguagesResponse
} from '$lib/sdk';
import type { TitlePage } from './title-page.svelte';
import { WantedLanguageList } from './wanted-languages.svelte';

/** The episode group ID that means TMDB's aired order. */
export const AIRED_ORDER = '';

/** TMDB's episode group types, as Catalog names them. */
export enum EpisodeGroupType {
	ORIGINAL_AIR_DATE = 'ORIGINAL_AIR_DATE',
	ABSOLUTE = 'ABSOLUTE',
	DVD = 'DVD',
	DIGITAL = 'DIGITAL',
	STORY_ARC = 'STORY_ARC',
	PRODUCTION = 'PRODUCTION',
	TV = 'TV'
}

export const EPISODE_GROUP_TYPE_LABELS: Record<string, string> = {
	[EpisodeGroupType.ORIGINAL_AIR_DATE]: 'Original air date',
	[EpisodeGroupType.ABSOLUTE]: 'Absolute',
	[EpisodeGroupType.DVD]: 'DVD',
	[EpisodeGroupType.DIGITAL]: 'Digital',
	[EpisodeGroupType.STORY_ARC]: 'Story arc',
	[EpisodeGroupType.PRODUCTION]: 'Production',
	[EpisodeGroupType.TV]: 'TV'
};

export interface TitleSettingsApi {
	episodeGroups(id: string): Promise<EpisodeGroupResponse[]>;
	selectEpisodeGroup(id: string, groupId: string): Promise<TitleResponse>;
	globalLanguages(): Promise<WantedLanguagesResponse>;
	setLanguages(id: string, languages: WantedLanguageEntry[] | null): Promise<TitleResponse>;
}

export const CATALOG_TITLE_SETTINGS_API: TitleSettingsApi = {
	episodeGroups: (id) => listEpisodeGroups(id),
	selectEpisodeGroup: (id, groupId) => selectEpisodeGroup(id, { groupId }),
	globalLanguages: () => getWantedLanguages(),
	setLanguages: (id, languages) => updateTitleLanguages(id, { languages })
};

/** A series' episode order: TMDB's aired order or one of its episode groups. */
export class EpisodeOrder {
	groups = $state<EpisodeGroupResponse[]>([]);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	saving = $state(false);
	readonly #page: TitlePage;
	readonly #api: TitleSettingsApi;

	constructor(page: TitlePage, api: TitleSettingsApi = CATALOG_TITLE_SETTINGS_API) {
		this.#page = page;
		this.#api = api;
	}

	get selected(): string {
		return this.#page.title?.episodeGroupId ?? AIRED_ORDER;
	}

	get selectedLabel(): string {
		if (this.selected === AIRED_ORDER) return 'Aired order (TMDB default)';
		return this.groups.find((group) => group.id === this.selected)?.name ?? this.selected;
	}

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		try {
			this.groups = await this.#api.episodeGroups(this.#page.id);
			this.status = LoadStatus.READY;
		} catch (error) {
			this.error = apiErrorMessage(error, 'Could not load the episode groups from TMDB.');
			this.status = LoadStatus.ERROR;
		}
	}

	async select(groupId: string): Promise<void> {
		if (groupId === this.selected) return;
		this.saving = true;
		try {
			this.#page.replace(await this.#api.selectEpisodeGroup(this.#page.id, groupId));
			toast.success('Episode order changed');
		} catch (error) {
			toast.error(apiErrorMessage(error, 'Could not change the episode order.'));
		} finally {
			this.saving = false;
		}
	}
}

export enum LanguageSource {
	GLOBAL = 'GLOBAL',
	CUSTOM = 'CUSTOM'
}

/** A Title's subtitle languages: the global list, or its own list. */
export class LanguageOverride {
	source = $state(LanguageSource.GLOBAL);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	saving = $state(false);
	readonly list = new WantedLanguageList();
	#global = $state<WantedLanguageEntry[]>([]);
	readonly #page: TitlePage;
	readonly #api: TitleSettingsApi;

	constructor(page: TitlePage, api: TitleSettingsApi = CATALOG_TITLE_SETTINGS_API) {
		this.#page = page;
		this.#api = api;
	}

	get #override(): WantedLanguageEntry[] | null {
		return this.#page.title?.languagesOverride ?? null;
	}

	/** The custom list differs from what is saved, or isn't saved yet. */
	get unsaved(): boolean {
		return this.source === LanguageSource.CUSTOM && (this.#override === null || this.list.dirty);
	}

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		try {
			const global = await this.#api.globalLanguages();
			this.#global = global.languages;
			const override = this.#override;
			this.source = override ? LanguageSource.CUSTOM : LanguageSource.GLOBAL;
			this.list.reset(override ?? global.languages, global.available);
			this.status = LoadStatus.READY;
		} catch (error) {
			this.error = apiErrorMessage(error);
			this.status = LoadStatus.ERROR;
		}
	}

	/** Custom starts from the global list and waits for Save; global applies at once. */
	async choose(source: LanguageSource): Promise<void> {
		if (source === this.source) return;
		this.source = source;
		if (source === LanguageSource.CUSTOM) {
			this.list.reset(this.#global);
			return;
		}
		if (this.#override !== null && !(await this.#send(null))) {
			this.source = LanguageSource.CUSTOM;
		}
	}

	async save(): Promise<void> {
		await this.#send(this.list.snapshot());
	}

	discard(): void {
		const override = this.#override;
		if (override) {
			this.list.reset(override);
			return;
		}
		this.source = LanguageSource.GLOBAL;
		this.list.reset(this.#global);
	}

	async #send(languages: WantedLanguageEntry[] | null): Promise<boolean> {
		this.saving = true;
		try {
			const title = await this.#api.setLanguages(this.#page.id, languages);
			this.#page.replace(title);
			this.list.reset(title.languagesOverride ?? this.#global);
			toast.success('Subtitle languages saved');
			return true;
		} catch (error) {
			toast.error(apiErrorMessage(error, 'Could not save the subtitle languages.'));
			return false;
		} finally {
			this.saving = false;
		}
	}
}
