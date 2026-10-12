import { describe, expect, it, vi } from 'vitest';
import { HttpError } from '@oazapfts/runtime';
import type { DownloadOptionsRequest, DownloadOptionsResponse } from '$lib/sdk';
import { patternError, timeoutsError } from './download-options';
import { DownloadOptionsManager, type DownloadOptionsApi } from './download-options-manager.svelte';

const BAD_REQUEST = 400;

const STORED: DownloadOptionsResponse = {
	blockedFilesEnabled: true,
	blockedPatterns: ['*.exe', '*.rar'],
	metadataTimeoutMinutes: 5,
	stalledLimitMinutes: 360,
	seedingMode: 'SEED_UNTIL_GOALS',
	seedRatio: 2,
	seedDays: 14
};

function fakeApi(): DownloadOptionsApi & { sent: DownloadOptionsRequest[] } {
	const sent: DownloadOptionsRequest[] = [];
	return {
		sent,
		get: vi.fn(async () => STORED),
		set: vi.fn(async (request: DownloadOptionsRequest) => {
			sent.push(request);
			return { ...request };
		})
	};
}

const notifier = () => ({ success: vi.fn(), error: vi.fn() });

describe('patternError', () => {
	it('accepts globs and compiling regexes, and rejects the rest', () => {
		expect(patternError('*.iso', STORED.blockedPatterns)).toBeUndefined();
		expect(patternError('regex:^setup\\.', STORED.blockedPatterns)).toBeUndefined();
		expect(patternError('', STORED.blockedPatterns)).toBeDefined();
		expect(patternError('*.EXE', STORED.blockedPatterns)).toContain('already');
		expect(patternError('regex:(', STORED.blockedPatterns)).toContain('compile');
		expect(patternError('extras/*.exe', STORED.blockedPatterns)).toContain('folders');
	});
});

describe('timeoutsError', () => {
	it('checks the ranges from the settings catalogue', () => {
		expect(timeoutsError({ metadataTimeoutMinutes: 5, stalledLimitHours: 6 })).toBeUndefined();
		expect(timeoutsError({ metadataTimeoutMinutes: 0, stalledLimitHours: 6 })).toContain(
			'Metadata'
		);
		expect(timeoutsError({ metadataTimeoutMinutes: 61, stalledLimitHours: 6 })).toContain(
			'Metadata'
		);
		expect(timeoutsError({ metadataTimeoutMinutes: 5, stalledLimitHours: 0.25 })).toContain(
			'Stalled'
		);
		expect(timeoutsError({ metadataTimeoutMinutes: 5, stalledLimitHours: null })).toContain(
			'Stalled'
		);
	});
});

describe('DownloadOptionsManager', () => {
	it('adds and removes patterns, sending the stored options with each change', async () => {
		const api = fakeApi();
		const manager = new DownloadOptionsManager(api, notifier());
		await manager.load();
		manager.timeouts.metadataTimeoutMinutes = 10;

		manager.newPattern = ' *.iso ';
		await manager.addPattern();
		await manager.removePattern('*.exe');

		expect(api.sent[0]).toEqual({ ...STORED, blockedPatterns: ['*.exe', '*.rar', '*.iso'] });
		expect(manager.saved?.blockedPatterns).toEqual(['*.rar', '*.iso']);
		expect(manager.newPattern).toBe('');
		expect(manager.timeouts.metadataTimeoutMinutes).toBe(10);
	});

	it('refuses a bad pattern without asking the server', async () => {
		const api = fakeApi();
		const manager = new DownloadOptionsManager(api, notifier());
		await manager.load();

		manager.newPattern = 'regex:(';
		await manager.addPattern();

		expect(manager.patternError).toBeDefined();
		expect(api.sent).toHaveLength(0);
	});

	it('saves timeouts in minutes and validates them first', async () => {
		const api = fakeApi();
		const notify = notifier();
		const manager = new DownloadOptionsManager(api, notify);
		await manager.load();
		expect(manager.timeouts.stalledLimitHours).toBe(6);

		manager.timeouts.stalledLimitHours = 100;
		await manager.saveTimeouts();
		expect(manager.timeoutsError).toBeDefined();
		expect(api.sent).toHaveLength(0);

		manager.timeouts = { metadataTimeoutMinutes: 15, stalledLimitHours: 12.5 };
		await manager.saveTimeouts();
		expect(api.sent[0]).toMatchObject({ metadataTimeoutMinutes: 15, stalledLimitMinutes: 750 });
		expect(notify.success).toHaveBeenCalled();
	});

	it('toggles the check and reports a rejected save', async () => {
		const api = fakeApi();
		const notify = notifier();
		const manager = new DownloadOptionsManager(api, notify);
		await manager.load();

		await manager.setEnabled(false);
		expect(api.sent[0].blockedFilesEnabled).toBe(false);

		vi.mocked(api.set).mockRejectedValueOnce(
			new HttpError(BAD_REQUEST, { detail: 'invalid pattern' }, new Headers())
		);
		await manager.setEnabled(true);
		expect(notify.error).toHaveBeenCalledWith('invalid pattern');
		expect(manager.saved?.blockedFilesEnabled).toBe(false);
	});
});
