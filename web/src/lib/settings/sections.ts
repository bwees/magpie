export enum SettingsSection {
	GENERAL = 'general',
	CONNECTIONS = 'connections',
	LIBRARY = 'library',
	QUALITY = 'quality',
	INDEXERS = 'indexers',
	DOWNLOADS = 'downloads',
	SUBTITLES = 'subtitles',
	TRANSCODE = 'transcode',
	AGENTS = 'agents',
	USERS = 'users',
	NOTIFICATIONS = 'notifications'
}

export interface SectionSpec {
	section: SettingsSection;
	label: string;
	/** Extra words the "Search settings…" box matches. */
	keywords: string;
}

export const SETTINGS_BASE = '/settings';

export const SETTINGS_SECTIONS: SectionSpec[] = [
	{
		section: SettingsSection.GENERAL,
		label: 'General',
		keywords: 'admin password backups restore'
	},
	{
		section: SettingsSection.CONNECTIONS,
		label: 'Connections',
		keywords: 'tmdb qbittorrent prowlarr jellyfin opensubtitles api key url'
	},
	{
		section: SettingsSection.LIBRARY,
		label: 'Library',
		keywords: 'root folders hardlink scan seeding'
	},
	{
		section: SettingsSection.QUALITY,
		label: 'Quality',
		keywords: 'profiles custom formats trash cutoff delay'
	},
	{ section: SettingsSection.INDEXERS, label: 'Indexers', keywords: 'prowlarr priority seeding' },
	{
		section: SettingsSection.DOWNLOADS,
		label: 'Downloads',
		keywords: 'blocked files patterns metadata stalled seeding goal'
	},
	{
		section: SettingsSection.SUBTITLES,
		label: 'Subtitles',
		keywords: 'languages opensubtitles whisper sync'
	},
	{
		section: SettingsSection.TRANSCODE,
		label: 'Transcode',
		keywords: 'hevc av1 preset bitrate cap skip backlog'
	},
	{
		section: SettingsSection.AGENTS,
		label: 'Agents',
		keywords: 'shared secret transfer gpu slots'
	},
	{ section: SettingsSection.USERS, label: 'Users', keywords: 'roles auto-approve jellyfin' },
	{
		section: SettingsSection.NOTIFICATIONS,
		label: 'Notifications',
		keywords: 'discord ntfy telegram shoutrrr url events'
	}
];

export function settingsPath(section: SettingsSection): string {
	return `${SETTINGS_BASE}/${section}`;
}

export function sectionFor(pathname: string): SectionSpec | undefined {
	return SETTINGS_SECTIONS.find(
		(spec) =>
			pathname === settingsPath(spec.section) ||
			pathname.startsWith(`${settingsPath(spec.section)}/`)
	);
}

/** Requesters see only their own notification settings. */
export function sectionsFor(admin: boolean): SectionSpec[] {
	return admin
		? SETTINGS_SECTIONS
		: SETTINGS_SECTIONS.filter((spec) => spec.section === SettingsSection.NOTIFICATIONS);
}

/** Sections whose name or keywords contain every word of query, in catalogue order. */
export function filterSections(query: string, sections = SETTINGS_SECTIONS): SectionSpec[] {
	const words = query.toLowerCase().split(/\s+/).filter(Boolean);
	return sections.filter((spec) => {
		const haystack = `${spec.label} ${spec.keywords}`.toLowerCase();
		return words.every((word) => haystack.includes(word));
	});
}
