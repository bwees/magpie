import { describe, expect, it, vi } from 'vitest';
import { HttpError } from '@oazapfts/runtime';
import type { EpisodeResponse, TitleResponse, UpdateTitleRequest } from '$lib/sdk';
import { MediaKind, PageStatus, TitlePage, type TitleApi } from './title-page.svelte';

vi.mock('svelte-sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const TITLE_ID = '01JTITLE';
const TINT = '120 80 40';

function conclave(change: Partial<TitleResponse> = {}): TitleResponse {
	return {
		id: TITLE_ID,
		name: 'Conclave',
		kind: MediaKind.MOVIE,
		monitored: true,
		images: { poster: `/api/catalog/images/${TITLE_ID}/POSTER`, backdrop: null, logo: null },
		seasons: [],
		...change
	} as TitleResponse;
}

const EPISODE_ID = '01JEPISODE';

function episode(id: string, season: number, number: number, monitored: boolean): EpisodeResponse {
	return { id, season, number, monitored, name: `Episode ${number}` } as EpisodeResponse;
}

/** Season 1 monitored, season 2 not. */
function severance(): TitleResponse {
	return conclave({
		name: 'Severance',
		kind: MediaKind.SERIES,
		seasons: [
			{
				number: 1,
				monitored: true,
				episodes: [episode(EPISODE_ID, 1, 1, true), episode('01JE2', 1, 2, true)]
			},
			{ number: 2, monitored: false, episodes: [episode('01JE3', 2, 1, false)] }
		]
	});
}

class FakeApi implements TitleApi {
	stored = conclave();
	failUpdate = false;
	removed: boolean[] = [];

	get = vi.fn(async (id: string) => {
		if (id !== TITLE_ID) {
			throw new HttpError(404, {}, new Headers());
		}
		return this.stored;
	});

	async update(_id: string, change: UpdateTitleRequest): Promise<TitleResponse> {
		if (this.failUpdate) {
			throw new HttpError(500, {}, new Headers());
		}
		this.stored = { ...this.stored, ...change } as TitleResponse;
		return this.stored;
	}

	updateSeason = vi.fn(async (_id: string, season: number, monitored: boolean) => {
		if (this.failUpdate) {
			throw new HttpError(500, {}, new Headers());
		}
		const seasons = this.stored.seasons.map((s) =>
			s.number === season
				? { ...s, monitored, episodes: s.episodes.map((e) => ({ ...e, monitored })) }
				: s
		);
		this.stored = { ...this.stored, seasons };
		return this.stored;
	});

	updateEpisode = vi.fn(async (_id: string, episodeId: string, monitored: boolean) => {
		if (this.failUpdate) {
			throw new HttpError(500, {}, new Headers());
		}
		const seasons = this.stored.seasons.map((s) => ({
			...s,
			episodes: s.episodes.map((e) => (e.id === episodeId ? { ...e, monitored } : e))
		}));
		this.stored = { ...this.stored, seasons };
		return this.stored;
	});

	async remove(_id: string, deleteFiles: boolean): Promise<unknown> {
		this.removed.push(deleteFiles);
		return {};
	}

	tint = vi.fn(async () => TINT);
}

describe('TitlePage', () => {
	it('loads the title and tints the hero from its poster', async () => {
		const api = new FakeApi();
		const page = new TitlePage(TITLE_ID, api);

		await page.load();

		expect(page.status).toBe(PageStatus.READY);
		expect(page.title?.name).toBe('Conclave');
		expect(api.tint).toHaveBeenCalledWith(`/api/catalog/images/${TITLE_ID}/POSTER?w=342`);
		expect(page.tint).toBe(TINT);
	});

	it('shows not found for an unknown title', async () => {
		const page = new TitlePage('missing', new FakeApi());
		await page.load();
		expect(page.status).toBe(PageStatus.NOT_FOUND);
	});

	it('switches monitoring at once and keeps the change when the server agrees', async () => {
		const page = new TitlePage(TITLE_ID, new FakeApi());
		await page.load();

		const pending = page.setMonitored(false);
		expect(page.title?.monitored).toBe(false);
		await pending;
		expect(page.title?.monitored).toBe(false);
	});

	it('puts the switch back when the server refuses', async () => {
		const api = new FakeApi();
		api.failUpdate = true;
		const page = new TitlePage(TITLE_ID, api);
		await page.load();

		await page.setMonitored(false);

		expect(page.title?.monitored).toBe(true);
	});

	it('writes a new quality profile through the title update and reverts on failure', async () => {
		const api = new FakeApi();
		api.stored = conclave({ profileId: 'p1080' });
		const page = new TitlePage(TITLE_ID, api);
		await page.load();

		await page.setProfile('p2160');
		expect(api.stored.profileId).toBe('p2160');

		api.failUpdate = true;
		await page.setProfile('p720');
		expect(page.title?.profileId).toBe('p2160');
	});

	it('monitors a season and its episodes at once', async () => {
		const api = new FakeApi();
		api.stored = severance();
		const page = new TitlePage(TITLE_ID, api);
		await page.load();

		const pending = page.setSeasonMonitored(2, true);
		const season = page.title?.seasons.find((s) => s.number === 2);
		expect(season?.monitored).toBe(true);
		expect(season?.episodes.every((e) => e.monitored)).toBe(true);
		await pending;

		expect(api.updateSeason).toHaveBeenCalledWith(TITLE_ID, 2, true);
		expect(page.isSeries).toBe(true);
	});

	it('puts season and episode switches back when the server refuses', async () => {
		const api = new FakeApi();
		api.stored = severance();
		api.failUpdate = true;
		const page = new TitlePage(TITLE_ID, api);
		await page.load();

		await page.setSeasonMonitored(1, false);
		await page.setEpisodeMonitored(EPISODE_ID, false);

		const season = page.title?.seasons.find((s) => s.number === 1);
		expect(season?.monitored).toBe(true);
		expect(season?.episodes.map((e) => e.monitored)).toEqual([true, true]);
	});

	it('monitors one episode', async () => {
		const api = new FakeApi();
		api.stored = severance();
		const page = new TitlePage(TITLE_ID, api);
		await page.load();

		await page.setEpisodeMonitored(EPISODE_ID, false);

		expect(page.title?.seasons[0].episodes[0].monitored).toBe(false);
		expect(page.title?.seasons[0].monitored).toBe(true);
	});

	it('removes the title with the chosen deleteFiles flag', async () => {
		const api = new FakeApi();
		const page = new TitlePage(TITLE_ID, api);
		await page.load();

		page.askToRemove();
		page.deleteFiles = true;
		expect(await page.remove()).toBe(true);

		expect(api.removed).toEqual([true]);
		expect(page.confirmingRemove).toBe(false);
	});
});
