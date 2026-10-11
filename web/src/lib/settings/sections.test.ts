import { describe, expect, it } from 'vitest';
import {
	filterSections,
	sectionFor,
	sectionsFor,
	SETTINGS_SECTIONS,
	SettingsSection,
	settingsPath
} from './sections';

describe('filterSections', () => {
	it('lists every section for an empty query', () => {
		expect(filterSections('  ')).toEqual(SETTINGS_SECTIONS);
	});

	it('matches names and keywords, ignoring case', () => {
		expect(filterSections('QBITTORRENT').map((s) => s.section)).toEqual([
			SettingsSection.CONNECTIONS
		]);
		expect(filterSections('backups').map((s) => s.section)).toEqual([SettingsSection.GENERAL]);
	});

	it('needs every word to match', () => {
		expect(filterSections('jellyfin roles').map((s) => s.section)).toEqual([SettingsSection.USERS]);
	});
});

describe('sectionsFor', () => {
	it('gives admins every section and requesters only Notifications', () => {
		expect(sectionsFor(true)).toEqual(SETTINGS_SECTIONS);
		expect(sectionsFor(false).map((s) => s.section)).toEqual([SettingsSection.NOTIFICATIONS]);
	});
});

describe('sectionFor', () => {
	it('finds the section a path belongs to', () => {
		expect(sectionFor(settingsPath(SettingsSection.CONNECTIONS))?.section).toBe(
			SettingsSection.CONNECTIONS
		);
		expect(sectionFor('/settings/unknown')).toBeUndefined();
	});
});
