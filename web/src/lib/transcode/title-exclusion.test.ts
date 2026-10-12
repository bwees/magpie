import { describe, expect, it, vi } from 'vitest';
import { LoadStatus } from '$lib/load-status';
import { ExclusionScope, TitleExclusion, type ExclusionApi } from './title-exclusion.svelte';

function fakeApi(refs: string[] = []) {
	return {
		list: vi.fn(async () => refs.map((ref) => ({ scope: ExclusionScope.TITLE, ref }))),
		exclude: vi.fn(async () => ({})),
		include: vi.fn(async () => undefined)
	} satisfies ExclusionApi;
}

function notify() {
	return { success: vi.fn(), error: vi.fn() };
}

describe('TitleExclusion', () => {
	it('reads whether the title is excluded', async () => {
		const exclusion = new TitleExclusion('T1', fakeApi(['T2', 'T1']), notify());
		await exclusion.load();
		expect(exclusion.status).toBe(LoadStatus.READY);
		expect(exclusion.excluded).toBe(true);
	});

	it('excludes and includes through the exclusions endpoint', async () => {
		const api = fakeApi();
		const exclusion = new TitleExclusion('T1', api, notify());
		await exclusion.load();

		await exclusion.set(true);
		expect(api.exclude).toHaveBeenCalledWith(ExclusionScope.TITLE, 'T1');
		expect(exclusion.excluded).toBe(true);

		await exclusion.set(false);
		expect(api.include).toHaveBeenCalledWith(ExclusionScope.TITLE, 'T1');
		expect(exclusion.excluded).toBe(false);
	});

	it('keeps the switch where it was when the change fails', async () => {
		const api = fakeApi();
		api.exclude.mockRejectedValueOnce(new Error('down'));
		const notifier = notify();
		const exclusion = new TitleExclusion('T1', api, notifier);
		await exclusion.load();

		await exclusion.set(true);

		expect(exclusion.excluded).toBe(false);
		expect(notifier.error).toHaveBeenCalled();
	});
});
