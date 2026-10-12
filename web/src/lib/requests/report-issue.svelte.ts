import { toast } from 'svelte-sonner';
import { apiErrorMessage, type IssueResponse, type SeasonResponse } from '$lib/sdk';
import { SDK_ISSUES_API, type IssuesApi } from './issues-api';
import { IssueKind, episodeCode } from './issues';

/** The episode picker's value for an issue about the whole title. */
export const WHOLE_TITLE = '';

export interface EpisodeOption {
	value: string;
	label: string;
}

/**
 * The released episodes of a series as picker options, after the whole-series option. A movie, or a
 * series with nothing released, has none.
 */
export function episodeOptions(seasons: SeasonResponse[]): EpisodeOption[] {
	const episodes = seasons.flatMap((season) =>
		season.episodes
			.filter((episode) => episode.released)
			.map((episode) => ({
				value: episode.id,
				label: `${episodeCode(episode.season, episode.number)} · ${episode.name}`
			}))
	);
	return episodes.length === 0
		? []
		: [{ value: WHOLE_TITLE, label: 'The whole series' }, ...episodes];
}

/** The title page's Report issue dialog: kind, optional episode and what's wrong. */
export class ReportIssue {
	open = $state(false);
	kind = $state(IssueKind.VIDEO);
	episodeId = $state(WHOLE_TITLE);
	text = $state('');
	submitting = $state(false);

	readonly #titleId: string;
	readonly #api: IssuesApi;

	constructor(titleId: string, api: IssuesApi = SDK_ISSUES_API) {
		this.#titleId = titleId;
		this.#api = api;
	}

	get valid(): boolean {
		return this.text.trim() !== '';
	}

	start(): void {
		this.kind = IssueKind.VIDEO;
		this.episodeId = WHOLE_TITLE;
		this.text = '';
		this.open = true;
	}

	async submit(): Promise<IssueResponse | undefined> {
		if (!this.valid) return undefined;
		this.submitting = true;
		try {
			const issue = await this.#api.report({
				titleId: this.#titleId,
				episodeId: this.episodeId === WHOLE_TITLE ? undefined : this.episodeId,
				kind: this.kind,
				text: this.text.trim()
			});
			this.open = false;
			return issue;
		} catch (error) {
			toast.error(apiErrorMessage(error));
			return undefined;
		} finally {
			this.submitting = false;
		}
	}
}
