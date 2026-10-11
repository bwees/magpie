/** Where search results render: the desktop popover or the phone's full-screen overlay. */
export enum SearchLayout {
	POPOVER = 'POPOVER',
	OVERLAY = 'OVERLAY'
}

export enum TitleTab {
	TIMELINE = 'TIMELINE',
	FILES = 'FILES',
	SETTINGS = 'SETTINGS'
}

/** Tailwind's lg breakpoint, where the title page's timeline moves to its own column. */
export const TIMELINE_COLUMN_QUERY = 'min-width: 1024px';
