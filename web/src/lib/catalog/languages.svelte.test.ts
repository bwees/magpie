import { describe, expect, it, vi } from 'vitest';
import { HttpError } from '@oazapfts/runtime';
import { LoadStatus } from '$lib/load-status';
import type {
	EpisodeGroupResponse,
	TitleResponse,
	WantedLanguageEntry,
	WantedLanguagesResponse
} from '$lib/sdk';
import { TitlePage, type TitleApi } from './title-page.svelte';
import {
	AIRED_ORDER,
	EpisodeOrder,
	LanguageOverride,
	LanguageSource,
	type TitleSettingsApi
} from './title-settings.svelte';
import {
	GlobalLanguages,
	LanguageFlag,
	WantedLanguageList,
	type GlobalLanguagesApi
} from './wanted-languages.svelte';

vi.mock('svelte-sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const TITLE_ID = '01JTITLE';
const GROUP_ID = 'dvd-order';
const OPTIONS = [
	{ code: 'en', name: 'English' },
	{ code: 'es', name: 'Spanish' },
	{ code: 'ja', name: 'Japanese' },
	{ code: 'de', name: 'German' }
];

const entry = (language: string): WantedLanguageEntry => ({
	language,
	forced: false,
	hearingImpaired: false
});

const GLOBAL: WantedLanguagesResponse = {
	languages: [entry('en'), entry('es')],
	available: OPTIONS
};

function listOf(...codes: string[]): WantedLanguageList {
	const list = new WantedLanguageList();
	list.reset(codes.map(entry), OPTIONS);
	return list;
}

const codes = (list: WantedLanguageList) => list.languages.map((e) => e.language);

describe('WantedLanguageList', () => {
	it('reorders, adds and flags languages and knows when it differs from what is saved', () => {
		const list = listOf('en', 'es');

		list.add('ja');
		list.move(2, 0);
		list.toggle(0, LanguageFlag.FORCED);

		expect(codes(list)).toEqual(['ja', 'en', 'es']);
		expect(list.languages[0].forced).toBe(true);
		expect(list.dirty).toBe(true);
		list.discard();
		expect(codes(list)).toEqual(['en', 'es']);
		expect(list.dirty).toBe(false);
	});

	it('moves the dragged row to where it is dropped', () => {
		const list = listOf('en', 'es', 'ja');
		list.startDrag(0);
		list.dropOn(2);
		expect(codes(list)).toEqual(['es', 'ja', 'en']);
		expect(list.dragging).toBeUndefined();
	});

	it('keeps at least one language and offers only the ones not chosen, by name', () => {
		const list = listOf('en');
		list.remove(0);
		expect(codes(list)).toEqual(['en']);
		expect(list.addable.map((o) => o.code)).toEqual(['de', 'ja', 'es']);
		list.add('en');
		expect(codes(list)).toEqual(['en']);
	});
});

class FakeLanguagesApi implements GlobalLanguagesApi {
	saved: WantedLanguageEntry[][] = [];
	get = vi.fn(async () => GLOBAL);
	save = vi.fn(async (languages: WantedLanguageEntry[]) => {
		this.saved.push(languages);
		return { languages, available: OPTIONS };
	});
}

describe('GlobalLanguages', () => {
	it('loads the global list and saves the edited one', async () => {
		const api = new FakeLanguagesApi();
		const languages = new GlobalLanguages(api);
		await languages.load();
		expect(languages.status).toBe(LoadStatus.READY);

		languages.list.move(1, 0);
		expect(await languages.save()).toBe(true);

		expect(api.saved).toEqual([[entry('es'), entry('en')]]);
		expect(languages.list.dirty).toBe(false);
	});
});

function series(change: Partial<TitleResponse> = {}): TitleResponse {
	return {
		id: TITLE_ID,
		name: 'Severance',
		kind: 'SERIES',
		episodeGroupId: AIRED_ORDER,
		languagesOverride: null,
		seasons: [],
		images: { poster: null, backdrop: null, logo: null },
		...change
	} as TitleResponse;
}

async function loadedPage(title: TitleResponse): Promise<TitlePage> {
	const api = { get: async () => title, tint: async () => undefined } as unknown as TitleApi;
	const page = new TitlePage(TITLE_ID, api);
	await page.load();
	return page;
}

class FakeSettingsApi implements TitleSettingsApi {
	groups: EpisodeGroupResponse[] = [
		{ id: GROUP_ID, name: 'DVD order', type: 'DVD', episodeCount: 10, groupCount: 2 }
	];
	languagesSent: (WantedLanguageEntry[] | null)[] = [];
	fail = false;

	episodeGroups = vi.fn(async () => this.groups);
	selectEpisodeGroup = vi.fn(async (_id: string, groupId: string) => {
		if (this.fail) throw new HttpError(503, {}, new Headers());
		return series({ episodeGroupId: groupId });
	});
	globalLanguages = vi.fn(async () => GLOBAL);
	setLanguages = vi.fn(async (_id: string, languages: WantedLanguageEntry[] | null) => {
		this.languagesSent.push(languages);
		if (this.fail) throw new HttpError(500, {}, new Headers());
		return series({ languagesOverride: languages });
	});
}

describe('EpisodeOrder', () => {
	it('lists the groups and switches the series to one', async () => {
		const page = await loadedPage(series());
		const api = new FakeSettingsApi();
		const order = new EpisodeOrder(page, api);
		await order.load();
		expect(order.selected).toBe(AIRED_ORDER);

		await order.select(GROUP_ID);

		expect(api.selectEpisodeGroup).toHaveBeenCalledWith(TITLE_ID, GROUP_ID);
		expect(order.selected).toBe(GROUP_ID);
		expect(order.selectedLabel).toBe('DVD order');
	});

	it('keeps the current order when TMDB fails', async () => {
		const page = await loadedPage(series());
		const api = new FakeSettingsApi();
		api.fail = true;
		const order = new EpisodeOrder(page, api);

		await order.select(GROUP_ID);

		expect(order.selected).toBe(AIRED_ORDER);
		expect(order.saving).toBe(false);
	});
});

describe('LanguageOverride', () => {
	it('starts a custom list from the global one and saves it as the override', async () => {
		const page = await loadedPage(series());
		const api = new FakeSettingsApi();
		const override = new LanguageOverride(page, api);
		await override.load();
		expect(override.source).toBe(LanguageSource.GLOBAL);

		await override.choose(LanguageSource.CUSTOM);
		expect(override.unsaved).toBe(true);
		override.list.add('ja');
		override.list.move(2, 0);
		await override.save();

		expect(api.languagesSent).toEqual([[entry('ja'), entry('en'), entry('es')]]);
		expect(page.title?.languagesOverride?.[0].language).toBe('ja');
		expect(override.unsaved).toBe(false);
	});

	it('clears the override when going back to the global list', async () => {
		const page = await loadedPage(series({ languagesOverride: [entry('ja')] }));
		const api = new FakeSettingsApi();
		const override = new LanguageOverride(page, api);
		await override.load();
		expect(override.source).toBe(LanguageSource.CUSTOM);
		expect(codes(override.list)).toEqual(['ja']);

		await override.choose(LanguageSource.GLOBAL);

		expect(api.languagesSent).toEqual([null]);
		expect(override.source).toBe(LanguageSource.GLOBAL);
		expect(codes(override.list)).toEqual(['en', 'es']);
	});

	it('stays custom when clearing fails', async () => {
		const page = await loadedPage(series({ languagesOverride: [entry('ja')] }));
		const api = new FakeSettingsApi();
		api.fail = true;
		const override = new LanguageOverride(page, api);
		await override.load();

		await override.choose(LanguageSource.GLOBAL);

		expect(override.source).toBe(LanguageSource.CUSTOM);
	});
});
