import { describe, expect, it } from 'vitest';
import { BOTTOM_TABS, NAV_ITEMS, SIDEBAR_GROUPS, navItemFor } from './nav';

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
