import type { Component } from 'svelte';
import House from '@lucide/svelte/icons/house';
import Inbox from '@lucide/svelte/icons/inbox';
import Film from '@lucide/svelte/icons/film';
import Tv from '@lucide/svelte/icons/tv';
import Activity from '@lucide/svelte/icons/activity';
import Settings from '@lucide/svelte/icons/settings';

export enum Route {
	HOME = '/',
	REQUESTS = '/requests',
	MOVIES = '/movies',
	TV = '/tv',
	ACTIVITY = '/activity',
	SETTINGS = '/settings'
}

export enum BadgeSource {
	ATTENTION = 'ATTENTION',
	PENDING_REQUESTS = 'PENDING_REQUESTS',
	LIVE_JOBS = 'LIVE_JOBS'
}

export interface NavItem {
	label: string;
	route: Route;
	icon: Component;
	badge?: BadgeSource;
}

export interface NavGroup {
	label: string;
	items: NavItem[];
}

export const NAV_ITEMS = {
	HOME: { label: 'Home', route: Route.HOME, icon: House, badge: BadgeSource.ATTENTION },
	REQUESTS: {
		label: 'Requests',
		route: Route.REQUESTS,
		icon: Inbox,
		badge: BadgeSource.PENDING_REQUESTS
	},
	MOVIES: { label: 'Movies', route: Route.MOVIES, icon: Film },
	TV: { label: 'TV', route: Route.TV, icon: Tv },
	ACTIVITY: {
		label: 'Activity',
		route: Route.ACTIVITY,
		icon: Activity,
		badge: BadgeSource.LIVE_JOBS
	},
	SETTINGS: { label: 'Settings', route: Route.SETTINGS, icon: Settings }
} as const satisfies Record<string, NavItem>;

export const SIDEBAR_GROUPS: NavGroup[] = [
	{ label: 'Browse', items: [NAV_ITEMS.HOME, NAV_ITEMS.REQUESTS] },
	{ label: 'Library', items: [NAV_ITEMS.MOVIES, NAV_ITEMS.TV] },
	{ label: 'System', items: [NAV_ITEMS.ACTIVITY, NAV_ITEMS.SETTINGS] }
];

export const BOTTOM_TABS: NavItem[] = [
	NAV_ITEMS.HOME,
	NAV_ITEMS.MOVIES,
	NAV_ITEMS.TV,
	NAV_ITEMS.REQUESTS,
	NAV_ITEMS.ACTIVITY
];

const ALL_ITEMS: NavItem[] = Object.values(NAV_ITEMS);

/** A nav item owns its route and everything nested under it, except Home which owns only "/". */
export function ownsPath(item: NavItem, pathname: string): boolean {
	if (item.route === Route.HOME) {
		return pathname === Route.HOME;
	}
	return pathname === item.route || pathname.startsWith(`${item.route}/`);
}

/** The nav item a path belongs to, or undefined for pages outside the nav such as title pages. */
export function navItemFor(pathname: string): NavItem | undefined {
	return ALL_ITEMS.find((item) => ownsPath(item, pathname));
}
