export enum ActivityTab {
	DOWNLOADS = 'downloads',
	IMPORTS = 'imports',
	GPU_JOBS = 'gpu-jobs',
	AGENTS = 'agents',
	HISTORY = 'history',
	BLOCKLIST = 'blocklist'
}

export const ACTIVITY_TABS: ActivityTab[] = Object.values(ActivityTab);

/** File managers see only the downloads of titles they may manage. */
export const FILE_MANAGER_ACTIVITY_TABS: ActivityTab[] = [ActivityTab.DOWNLOADS];

export function activityTabsFor(admin: boolean): ActivityTab[] {
	return admin ? ACTIVITY_TABS : FILE_MANAGER_ACTIVITY_TABS;
}

/** The query parameter that opens Activity on a tab, e.g. ?tab=imports. */
export const ACTIVITY_TAB_QUERY = 'tab';

export const TAB_LABELS: Record<ActivityTab, string> = {
	[ActivityTab.DOWNLOADS]: 'Downloads',
	[ActivityTab.IMPORTS]: 'Imports',
	[ActivityTab.GPU_JOBS]: 'GPU jobs',
	[ActivityTab.AGENTS]: 'Agents',
	[ActivityTab.HISTORY]: 'History',
	[ActivityTab.BLOCKLIST]: 'Blocklist'
};
