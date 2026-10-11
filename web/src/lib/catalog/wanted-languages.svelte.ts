import { toast } from 'svelte-sonner';
import { LoadStatus } from '$lib/load-status';
import {
	apiErrorMessage,
	getWantedLanguages,
	updateWantedLanguages,
	type LanguageOptionResponse,
	type WantedLanguageEntry,
	type WantedLanguagesResponse
} from '$lib/sdk';

/** A wanted-language list never goes below this many entries. */
export const MIN_WANTED_LANGUAGES = 1;

export enum LanguageFlag {
	FORCED = 'forced',
	HEARING_IMPAIRED = 'hearingImpaired'
}

function sameList(a: WantedLanguageEntry[], b: WantedLanguageEntry[]): boolean {
	return (
		a.length === b.length &&
		a.every(
			(entry, i) =>
				entry.language === b[i].language &&
				entry.forced === b[i].forced &&
				entry.hearingImpaired === b[i].hearingImpaired
		)
	);
}

/** An ordered wanted-language list being edited, compared with what is saved. */
export class WantedLanguageList {
	languages = $state<WantedLanguageEntry[]>([]);
	saved = $state<WantedLanguageEntry[]>([]);
	options = $state<LanguageOptionResponse[]>([]);
	/** The row being dragged, by position. */
	dragging = $state<number | undefined>();

	get dirty(): boolean {
		return !sameList(this.languages, this.saved);
	}

	get canRemove(): boolean {
		return this.languages.length > MIN_WANTED_LANGUAGES;
	}

	/** Languages not in the list yet, by name. */
	get addable(): LanguageOptionResponse[] {
		return this.options
			.filter((option) => !this.languages.some((entry) => entry.language === option.code))
			.sort((a, b) => a.name.localeCompare(b.name));
	}

	nameOf(code: string): string {
		return this.options.find((option) => option.code === code)?.name ?? code;
	}

	reset(languages: WantedLanguageEntry[], options = this.options): void {
		this.options = options;
		this.saved = languages.map((entry) => ({ ...entry }));
		this.languages = languages.map((entry) => ({ ...entry }));
	}

	discard(): void {
		this.languages = this.saved.map((entry) => ({ ...entry }));
	}

	add(code: string): void {
		if (this.languages.some((entry) => entry.language === code)) return;
		this.languages.push({ language: code, forced: false, hearingImpaired: false });
	}

	remove(position: number): void {
		if (this.canRemove) this.languages.splice(position, 1);
	}

	move(from: number, to: number): void {
		if (from === to || to < 0 || to >= this.languages.length) return;
		const [entry] = this.languages.splice(from, 1);
		this.languages.splice(to, 0, entry);
	}

	toggle(position: number, flag: LanguageFlag): void {
		const entry = this.languages[position];
		if (entry) entry[flag] = !entry[flag];
	}

	startDrag(position: number): void {
		this.dragging = position;
	}

	/** Moves the dragged row to position. */
	dropOn(position: number): void {
		if (this.dragging !== undefined) this.move(this.dragging, position);
		this.dragging = undefined;
	}

	/** A plain copy for sending to the server. */
	snapshot(): WantedLanguageEntry[] {
		return this.languages.map((entry) => ({ ...entry }));
	}
}

export interface GlobalLanguagesApi {
	get(): Promise<WantedLanguagesResponse>;
	save(languages: WantedLanguageEntry[]): Promise<WantedLanguagesResponse>;
}

export const CATALOG_LANGUAGES_API: GlobalLanguagesApi = {
	get: () => getWantedLanguages(),
	save: (languages) => updateWantedLanguages({ languages })
};

/** The global wanted languages, edited in Settings → Subtitles and the setup wizard. */
export class GlobalLanguages {
	status = $state(LoadStatus.LOADING);
	error = $state('');
	saving = $state(false);
	readonly list = new WantedLanguageList();
	readonly #api: GlobalLanguagesApi;

	constructor(api: GlobalLanguagesApi = CATALOG_LANGUAGES_API) {
		this.#api = api;
	}

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		try {
			const response = await this.#api.get();
			this.list.reset(response.languages, response.available);
			this.status = LoadStatus.READY;
		} catch (error) {
			this.error = apiErrorMessage(error);
			this.status = LoadStatus.ERROR;
		}
	}

	/** Saves the list; true when the server accepted it. */
	async save(): Promise<boolean> {
		this.saving = true;
		try {
			const response = await this.#api.save(this.list.snapshot());
			this.list.reset(response.languages, response.available);
			toast.success('Wanted languages saved');
			return true;
		} catch (error) {
			toast.error(apiErrorMessage(error, 'Could not save the wanted languages.'));
			return false;
		} finally {
			this.saving = false;
		}
	}
}
