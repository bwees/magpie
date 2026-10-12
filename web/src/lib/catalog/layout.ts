/** Where search results render: the desktop popover or the phone's full-screen overlay. */
export enum SearchLayout {
	POPOVER = 'POPOVER',
	OVERLAY = 'OVERLAY'
}

export enum TitleTab {
	EPISODES = 'EPISODES',
	TIMELINE = 'TIMELINE',
	FILES = 'FILES',
	SETTINGS = 'SETTINGS'
}

/** The library poster grid: about three per row on phones, wider cards from sm up. */
export const POSTER_GRID =
	'grid grid-cols-[repeat(auto-fill,minmax(100px,1fr))] gap-3 sm:grid-cols-[repeat(auto-fill,minmax(140px,1fr))] sm:gap-4';

/** How many skeleton posters a loading grid shows. */
export const SKELETON_POSTERS = 12;

/** Tailwind's lg breakpoint, where the title page's timeline moves to its own column. */
export const TIMELINE_COLUMN_QUERY = 'min-width: 1024px';
