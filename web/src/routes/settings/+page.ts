import { redirect } from '@sveltejs/kit';
import { SettingsSection, settingsPath } from '$lib/settings/sections';

const TEMPORARY_REDIRECT = 307;

export function load() {
	redirect(TEMPORARY_REDIRECT, settingsPath(SettingsSection.GENERAL));
}
