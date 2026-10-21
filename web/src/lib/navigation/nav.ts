import type { Component } from 'svelte';
import House from '@lucide/svelte/icons/house';
import Inbox from '@lucide/svelte/icons/inbox';
import Film from '@lucide/svelte/icons/film';
import Tv from '@lucide/svelte/icons/tv';
import Activity from '@lucide/svelte/icons/activity';
import Settings from '@lucide/svelte/icons/settings';
import Search from '@lucide/svelte/icons/search';
import { SettingsSection, settingsPath } from '$lib/settings/sections';

export enum Route {
	HOME = '/',
	REQUESTS = '/requests',
	MOVIES = '/movies',
	TV = '/tv',
	ACTIVITY = '/activity',
	SETTINGS = '/settings',
	ABOUT = '/about'
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

/** A bottom tab that opens something in place instead of navigating. */
export enum NavAction {
	SEARCH = 'SEARCH'
}

export interface ActionTab {
	label: string;
	icon: Component;
	action: NavAction;
}

export type BottomTab = NavItem | ActionTab;

export function isActionTab(tab: BottomTab): tab is ActionTab {
	return 'action' in tab;
}

export const SEARCH_TAB: ActionTab = { label: 'Search', icon: Search, action: NavAction.SEARCH };

/** Requesters see Home, Requests and title pages only. */
export const REQUESTER_SIDEBAR_GROUPS: NavGroup[] = [
	{ label: 'Browse', items: [NAV_ITEMS.HOME, NAV_ITEMS.REQUESTS] }
];

export const REQUESTER_BOTTOM_TABS: BottomTab[] = [NAV_ITEMS.HOME, SEARCH_TAB, NAV_ITEMS.REQUESTS];

/** File managers also get Activity, which shows them only their titles' downloads. */
export const FILE_MANAGER_SIDEBAR_GROUPS: NavGroup[] = [
	...REQUESTER_SIDEBAR_GROUPS,
	{ label: 'System', items: [NAV_ITEMS.ACTIVITY] }
];

export const FILE_MANAGER_BOTTOM_TABS: BottomTab[] = [...REQUESTER_BOTTOM_TABS, NAV_ITEMS.ACTIVITY];

/** Which navigation a signed-in user gets. */
export enum NavRole {
	ADMIN = 'ADMIN',
	FILE_MANAGER = 'FILE_MANAGER',
	REQUESTER = 'REQUESTER'
}

const SIDEBAR_GROUPS_BY_ROLE: Record<NavRole, NavGroup[]> = {
	[NavRole.ADMIN]: SIDEBAR_GROUPS,
	[NavRole.FILE_MANAGER]: FILE_MANAGER_SIDEBAR_GROUPS,
	[NavRole.REQUESTER]: REQUESTER_SIDEBAR_GROUPS
};

const BOTTOM_TABS_BY_ROLE: Record<NavRole, BottomTab[]> = {
	[NavRole.ADMIN]: BOTTOM_TABS,
	[NavRole.FILE_MANAGER]: FILE_MANAGER_BOTTOM_TABS,
	[NavRole.REQUESTER]: REQUESTER_BOTTOM_TABS
};

/** Settings → Notifications is the one settings page every user has. */
export const NOTIFICATION_SETTINGS_PATH = settingsPath(SettingsSection.NOTIFICATIONS);

const ADMIN_ONLY_ITEMS: NavItem[] = [
	NAV_ITEMS.MOVIES,
	NAV_ITEMS.TV,
	NAV_ITEMS.ACTIVITY,
	NAV_ITEMS.SETTINGS
];

export function sidebarGroupsFor(role: NavRole): NavGroup[] {
	return SIDEBAR_GROUPS_BY_ROLE[role];
}

export function bottomTabsFor(role: NavRole): BottomTab[] {
	return BOTTOM_TABS_BY_ROLE[role];
}

/** Whether only admins may open pathname. */
export function isAdminOnly(pathname: string): boolean {
	if (pathname === NOTIFICATION_SETTINGS_PATH) {
		return false;
	}
	return ADMIN_ONLY_ITEMS.some((item) => ownsPath(item, pathname));
}

/** File managers open the Activity page itself, but not its admin sub-pages such as agents. */
export function mayOpen(pathname: string, role: NavRole): boolean {
	switch (role) {
		case NavRole.ADMIN:
			return true;
		case NavRole.FILE_MANAGER:
			return pathname === Route.ACTIVITY || !isAdminOnly(pathname);
		case NavRole.REQUESTER:
			return !isAdminOnly(pathname);
	}
}

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
