import { describe, expect, it } from 'vitest';
import {
	BOTTOM_TABS,
	NAV_ITEMS,
	NavRole,
	NOTIFICATION_SETTINGS_PATH,
	SEARCH_TAB,
	SIDEBAR_GROUPS,
	bottomTabsFor,
	isActionTab,
	isAdminOnly,
	navItemFor,
	sidebarGroupsFor
} from './nav';

describe('isAdminOnly', () => {
	it('keeps the library, activity and settings for admins', () => {
		for (const path of ['/movies', '/tv/12', '/activity', '/settings', '/settings/users']) {
			expect(isAdminOnly(path)).toBe(true);
		}
	});

	it('leaves Home, Requests, title pages and notification settings to everyone', () => {
		for (const path of ['/', '/requests', '/titles/42', NOTIFICATION_SETTINGS_PATH]) {
			expect(isAdminOnly(path)).toBe(false);
		}
	});
});

describe('navItemFor', () => {
	it('matches each nav route exactly', () => {
		for (const item of Object.values(NAV_ITEMS)) {
			expect(navItemFor(item.route)).toBe(item);
		}
	});

	it('keeps nested pages under their section', () => {
		expect(navItemFor('/settings/general')).toBe(NAV_ITEMS.SETTINGS);
	});

	it('does not treat every path as Home', () => {
		expect(navItemFor('/titles/42')).toBeUndefined();
	});

	it('does not match a route that only shares a prefix', () => {
		expect(navItemFor('/tvshows')).toBeUndefined();
	});
});

describe('navigation layout', () => {
	it('groups the sidebar as Browse, Library, System', () => {
		expect(SIDEBAR_GROUPS.map((group) => [group.label, group.items.map((i) => i.label)])).toEqual([
			['Browse', ['Home', 'Requests']],
			['Library', ['Movies', 'TV']],
			['System', ['Activity', 'Settings']]
		]);
	});

	it('gives requesters Home and Requests, with Search between them on phones', () => {
		expect(
			sidebarGroupsFor(NavRole.REQUESTER).flatMap((group) => group.items.map((i) => i.label))
		).toEqual(['Home', 'Requests']);
		expect(bottomTabsFor(NavRole.REQUESTER).map((tab) => tab.label)).toEqual([
			'Home',
			'Search',
			'Requests'
		]);
		expect(bottomTabsFor(NavRole.REQUESTER).filter(isActionTab)).toEqual([SEARCH_TAB]);
		expect(sidebarGroupsFor(NavRole.ADMIN)).toBe(SIDEBAR_GROUPS);
		expect(bottomTabsFor(NavRole.ADMIN)).toBe(BOTTOM_TABS);
	});

	it('adds Activity for file managers', () => {
		expect(
			sidebarGroupsFor(NavRole.FILE_MANAGER).flatMap((group) => group.items.map((i) => i.label))
		).toEqual(['Home', 'Requests', 'Activity']);
		expect(bottomTabsFor(NavRole.FILE_MANAGER).map((tab) => tab.label)).toEqual([
			'Home',
			'Search',
			'Requests',
			'Activity'
		]);
	});

	it('puts five tabs on phones and leaves Settings to the avatar menu', () => {
		expect(BOTTOM_TABS.map((tab) => tab.label)).toEqual([
			'Home',
			'Movies',
			'TV',
			'Requests',
			'Activity'
		]);
	});
});
