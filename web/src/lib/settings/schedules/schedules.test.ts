import { describe, expect, it, vi } from 'vitest';
import type { SchedulesRequest, SchedulesResponse } from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import { schedulesError, type SchedulesDraft } from './schedules';
import { SchedulesManager } from './schedules-manager.svelte';

const STORED: SchedulesResponse = {
	rssIntervalMinutes: 15,
	missingSweepDays: 1,
	cutoffSweepDays: 7,
	pendingCheckMinutes: 1
};

const VALID: SchedulesDraft = {
	rssIntervalMinutes: 15,
	missingSweepDays: 1,
	cutoffSweepDays: 7
};

describe('schedulesError', () => {
	it('checks the ranges from the settings catalogue', () => {
		expect(schedulesError(VALID)).toBeUndefined();
		expect(schedulesError({ ...VALID, rssIntervalMinutes: 5 })).toContain('RSS');
		expect(schedulesError({ ...VALID, missingSweepDays: 8 })).toContain('missing');
		expect(schedulesError({ ...VALID, cutoffSweepDays: null })).toContain('cutoff');
		expect(schedulesError({ ...VALID, missingSweepDays: 1.5 })).toContain('missing');
	});
});

describe('SchedulesManager', () => {
	it('loads, refuses an invalid draft and saves a valid one', async () => {
		const set = vi.fn(async (request: SchedulesRequest) => ({
			...request,
			pendingCheckMinutes: 1
		}));
		const notify = { success: vi.fn(), error: vi.fn() };
		const manager = new SchedulesManager({ get: vi.fn(async () => STORED), set }, notify);

		await manager.load();
		expect(manager.status).toBe(LoadStatus.READY);
		expect(manager.draft.rssIntervalMinutes).toBe(15);

		manager.draft.rssIntervalMinutes = 200;
		await manager.save();
		expect(manager.draftError).toContain('RSS');
		expect(set).not.toHaveBeenCalled();

		manager.draft.rssIntervalMinutes = 30;
		await manager.save();
		expect(set).toHaveBeenCalledWith({
			rssIntervalMinutes: 30,
			missingSweepDays: 1,
			cutoffSweepDays: 7
		});
		expect(manager.saved?.rssIntervalMinutes).toBe(30);
		expect(notify.success).toHaveBeenCalledOnce();
	});
});
