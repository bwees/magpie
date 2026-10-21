import { describe, expect, it, vi } from 'vitest';
import { TitleAccess } from './title-access.svelte';

vi.mock('$lib/sdk', () => ({ getTitleAccess: vi.fn() }));

describe('TitleAccess', () => {
	it('takes the server’s answer for the title', async () => {
		const api = vi.fn().mockResolvedValue({ manageFiles: true });
		const access = new TitleAccess('t1', false, api);

		await access.load();

		expect(api).toHaveBeenCalledWith('t1');
		expect(access.manageFiles).toBe(true);
	});

	it('keeps the admin default when the check fails', async () => {
		const access = new TitleAccess('t1', true, vi.fn().mockRejectedValue(new Error('offline')));

		await access.load();

		expect(access.manageFiles).toBe(true);
	});

	it('keeps a requester locked out when the check fails', async () => {
		const access = new TitleAccess('t1', false, vi.fn().mockRejectedValue(new Error('offline')));

		await access.load();

		expect(access.manageFiles).toBe(false);
	});
});
