import { describe, expect, it, vi } from 'vitest';
import { HttpError } from '@oazapfts/runtime';
import type { TitleResponse, UpdateTitleRequest } from '$lib/sdk';
import { PageStatus, TitlePage, type TitleApi } from './title-page.svelte';

vi.mock('svelte-sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const TITLE_ID = '01JTITLE';
const TINT = '120 80 40';

function conclave(change: Partial<TitleResponse> = {}): TitleResponse {
	return {
		id: TITLE_ID,
		name: 'Conclave',
		monitored: true,
		images: { poster: `/api/catalog/images/${TITLE_ID}/POSTER`, backdrop: null, logo: null },
		...change
	} as TitleResponse;
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
