import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	deleteSubtitle,
	downloadSubtitle,
	getFileSubtitles,
	listSubtitleCandidates,
	searchFileSubtitles,
	type CandidateResponse,
	type FileSubtitlesResponse
} from '$lib/sdk';
import type { LiveConnection } from '$lib/live/live-connection';
import { LoadStatus } from '$lib/load-status';
import { SUBTITLE_EVENTS, trackLabel, wantedLanguages } from './subtitles';

export interface FileSubtitlesApi {
	get: (file: string) => Promise<FileSubtitlesResponse>;
	search: (file: string) => Promise<FileSubtitlesResponse>;
	candidates: (file: string, language: string, forced: boolean) => Promise<CandidateResponse[]>;
	download: (
		file: string,
		fileId: number,
		language: string,
		forced: boolean
	) => Promise<FileSubtitlesResponse>;
	remove: (subtitle: string) => Promise<unknown>;
}

export interface Notifier {
	success: (message: string) => void;
	error: (message: string) => void;
}

export type SubtitlesLive = Pick<LiveConnection, 'on' | 'onReset'>;

const sdkApi: FileSubtitlesApi = {
	get: (file) => getFileSubtitles(file),
	search: (file) => searchFileSubtitles(file),
	candidates: (file, language, forced) => listSubtitleCandidates(file, language, { forced }),
	download: (file, fileId, language, forced) =>
		downloadSubtitle(file, { fileId, language, forced }),
	remove: (subtitle) => deleteSubtitle(subtitle)
};

const FALLBACK_LANGUAGE = 'en';

/** One media file's subtitles on the Files tab, with the manual search and the candidate picker. */
export class FileSubtitles {
	data = $state<FileSubtitlesResponse>();
	status = $state(LoadStatus.LOADING);
	error = $state('');
	searching = $state(false);
	removing = $state<string>();

	pickerOpen = $state(false);
	pickerLanguage = $state(FALLBACK_LANGUAGE);
	pickerForced = $state(false);
	candidates = $state<CandidateResponse[]>([]);
	candidatesStatus = $state(LoadStatus.LOADING);
	candidatesError = $state('');
	downloading = $state<number>();

	readonly #file: string;
	readonly #api: FileSubtitlesApi;
	readonly #notify: Notifier;

	constructor(file: string, api: FileSubtitlesApi = sdkApi, notify: Notifier = toast) {
		this.#file = file;
		this.#api = api;
		this.#notify = notify;
	}

	/** The wanted languages of the file, for the picker; English when none are known yet. */
	get languages(): string[] {
		const wanted = wantedLanguages(this.data?.coverage ?? []);
		return wanted.length > 0 ? wanted : [FALLBACK_LANGUAGE];
	}

	async load(): Promise<void> {
		try {
			this.data = await this.#api.get(this.#file);
			this.status = LoadStatus.READY;
			this.error = '';
		} catch (error) {
			if (this.status !== LoadStatus.READY) this.status = LoadStatus.ERROR;
			this.error = apiErrorMessage(error, 'Could not load the subtitles.');
		}
	}

	async searchNow(): Promise<void> {
		this.searching = true;
		try {
			this.data = await this.#api.search(this.#file);
			this.#notify.success('Searching for missing subtitles');
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not start the search.'));
		} finally {
			this.searching = false;
		}
	}

	async remove(subtitle: string): Promise<void> {
		this.removing = subtitle;
		try {
			await this.#api.remove(subtitle);
			await this.load();
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not remove the subtitle.'));
		} finally {
			this.removing = undefined;
		}
	}

	openPicker(language = this.languages[0], forced = false): void {
		this.pickerLanguage = language;
		this.pickerForced = forced;
		this.pickerOpen = true;
		void this.loadCandidates();
	}

	closePicker(): void {
		this.pickerOpen = false;
	}

	async choose(language: string, forced: boolean): Promise<void> {
		this.pickerLanguage = language;
		this.pickerForced = forced;
		await this.loadCandidates();
	}

	async loadCandidates(): Promise<void> {
		const language = this.pickerLanguage;
		const forced = this.pickerForced;
		this.candidatesStatus = LoadStatus.LOADING;
		try {
			const found = await this.#api.candidates(this.#file, language, forced);
			if (language !== this.pickerLanguage || forced !== this.pickerForced) {
				return;
			}
			this.candidates = found;
			this.candidatesStatus = LoadStatus.READY;
			this.candidatesError = '';
		} catch (error) {
			this.candidatesStatus = LoadStatus.ERROR;
			this.candidatesError = apiErrorMessage(error, 'Could not search OpenSubtitles.');
		}
	}

	async download(candidate: CandidateResponse): Promise<void> {
		this.downloading = candidate.fileId;
		try {
			this.data = await this.#api.download(
				this.#file,
				candidate.fileId,
				this.pickerLanguage,
				this.pickerForced
			);
			this.#notify.success(
				`${trackLabel({ language: this.pickerLanguage, forced: this.pickerForced })} subtitle saved`
			);
			this.pickerOpen = false;
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not download the subtitle.'));
		} finally {
			this.downloading = undefined;
		}
	}

	/** Reloads when a subtitle event names this file. Returns a cleanup. */
	watch(live: SubtitlesLive): () => void {
		const stops = SUBTITLE_EVENTS.map((type) =>
			live.on(type, (m) => {
				if ((m.data as { mediaFile?: string }).mediaFile === this.#file) void this.load();
			})
		);
		stops.push(live.onReset(() => void this.load()));
		return () => stops.forEach((stop) => stop());
	}
}
