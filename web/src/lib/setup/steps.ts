import { Service } from '$lib/connections/services';

/** The first-run steps in 10-ux order. */
export enum WizardStep {
	LOCAL_ADMIN = 'LOCAL_ADMIN',
	TMDB = 'TMDB',
	QBITTORRENT = 'QBITTORRENT',
	PROWLARR = 'PROWLARR',
	JELLYFIN = 'JELLYFIN',
	LIBRARY_PATHS = 'LIBRARY_PATHS',
	QUALITY_PRESET = 'QUALITY_PRESET',
	WANTED_LANGUAGES = 'WANTED_LANGUAGES',
	OPENSUBTITLES = 'OPENSUBTITLES',
	TRANSCODE_TARGET = 'TRANSCODE_TARGET',
	ARR_IMPORT = 'ARR_IMPORT'
}

export enum StepKind {
	ACCOUNT = 'ACCOUNT',
	CONNECTION = 'CONNECTION',
	QUALITY_PRESET = 'QUALITY_PRESET',
	LANGUAGES = 'LANGUAGES',
	ARR_IMPORT = 'ARR_IMPORT',
	/** Owned by a later slice, which replaces the placeholder. */
	COMING_SOON = 'COMING_SOON'
}

export enum StepOutcome {
	PENDING = 'PENDING',
	DONE = 'DONE',
	SKIPPED = 'SKIPPED'
}

export interface StepSpec {
	step: WizardStep;
	label: string;
	description: string;
	kind: StepKind;
	service?: Service;
}

export const WIZARD_STEPS: StepSpec[] = [
	{
		step: WizardStep.LOCAL_ADMIN,
		label: 'Local admin',
		description: 'The admin account that always works, even when Jellyfin is down.',
		kind: StepKind.ACCOUNT
	},
	{
		step: WizardStep.TMDB,
		label: 'TMDB key',
		description: 'Magpie looks up titles on TMDB.',
		kind: StepKind.CONNECTION,
		service: Service.TMDB
	},
	{
		step: WizardStep.QBITTORRENT,
		label: 'qBittorrent',
		description: 'Where downloads run. Magpie creates its own category.',
		kind: StepKind.CONNECTION,
		service: Service.QBITTORRENT
	},
	{
		step: WizardStep.PROWLARR,
		label: 'Prowlarr',
		description: 'Your indexers come from Prowlarr.',
		kind: StepKind.CONNECTION,
		service: Service.PROWLARR
	},
	{
		step: WizardStep.JELLYFIN,
		label: 'Jellyfin',
		description: 'Your users sign in with Jellyfin, and Jellyfin serves the library.',
		kind: StepKind.CONNECTION,
		service: Service.JELLYFIN
	},
	{
		step: WizardStep.LIBRARY_PATHS,
		label: 'Library paths',
		description: 'The Movies and TV root folders, with a hardlink check.',
		kind: StepKind.COMING_SOON
	},
	{
		step: WizardStep.QUALITY_PRESET,
		label: 'Quality preset',
		description: 'The default quality for new requests: 720p, 1080p or 2160p.',
		kind: StepKind.QUALITY_PRESET
	},
	{
		step: WizardStep.WANTED_LANGUAGES,
		label: 'Wanted languages',
		description:
			'Subtitles are found for these languages, and transcodes keep their audio and subtitle tracks. The first is most wanted.',
		kind: StepKind.LANGUAGES
	},
	{
		step: WizardStep.OPENSUBTITLES,
		label: 'OpenSubtitles',
		description: 'Your OpenSubtitles.com login for subtitle downloads.',
		kind: StepKind.CONNECTION,
		service: Service.OPENSUBTITLES
	},
	{
		step: WizardStep.TRANSCODE_TARGET,
		label: 'Transcode target',
		description: 'The codec and quality every file is converted to.',
		kind: StepKind.COMING_SOON
	},
	{
		step: WizardStep.ARR_IMPORT,
		label: 'Import from Sonarr/Radarr',
		description: 'Bring over your existing library and history. Optional.',
		kind: StepKind.ARR_IMPORT
	}
];
