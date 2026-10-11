import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ProfileResponse, TrashStatusResponse } from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import {
	formatSyncedAt,
	presetProfile,
	profileDraft,
	profileDraftError,
	profileRequest,
	QualityPreset,
	settingsDraftError
} from './quality';
import { QualitySettingsManager, SYNC_POLL_MS } from './quality-manager.svelte';

const sdk = vi.hoisted(() => ({
	listProfiles: vi.fn(),
	getAcquisitionSettings: vi.fn(),
	updateAcquisitionSettings: vi.fn(),
	getTrashStatus: vi.fn(),
	syncTrash: vi.fn(),
	listCustomFormats: vi.fn(),
	blockTerm: vi.fn(),
	deleteCustomFormat: vi.fn(),
	createProfileFromPreset: vi.fn(),
	getProfile: vi.fn(),
	updateProfile: vi.fn(),
	deleteProfile: vi.fn()
}));

vi.mock('$lib/sdk', () => ({
	...sdk,
	apiErrorMessage: (error: { data?: { detail?: string } }, fallback: string) =>
		error?.data?.detail ?? fallback
}));

const P1080 = { id: 'p1080', name: '1080p', preset: QualityPreset.P1080, upgradeAllowed: true };
const P720 = { id: 'p720', name: '720p', preset: QualityPreset.P720, upgradeAllowed: true };
const SETTINGS = {
	defaultProfileId: 'p1080',
	defaultDelayMinutes: 0,
	settleWindowSeconds: 30,
	trashSyncEnabled: true
};
const NEVER_SYNCED: TrashStatusResponse = { enabled: true };

const PROFILE: ProfileResponse = {
	id: 'p1080',
	name: '1080p',
	preset: QualityPreset.P1080,
	upgradeAllowed: true,
	cutoff: 0,
	minFormatScore: 0,
	cutoffFormatScore: 0,
	minUpgradeFormatScore: 1,
	propersMode: 'PREFER_AND_UPGRADE',
	items: [
		{
			group: { id: 1000, name: '1080p' },
			qualities: [
				{ id: 3, name: 'WEBDL-1080p', source: 'WEBDL', resolution: 1080 },
				{ id: 7, name: 'Bluray-1080p', source: 'BLURAY', resolution: 1080 }
			],
			allowed: true,
			minMbPerMin: 12.5,
			preferredMbPerMin: 1999,
			maxMbPerMin: 2000
		},
		{
			qualities: [{ id: 9, name: 'HDTV-1080p', source: 'TV', resolution: 1080 }],
			allowed: false,
			minMbPerMin: 0,
			preferredMbPerMin: 0,
			maxMbPerMin: 0
		}
	],
	formatScores: [{ formatId: 'f1', name: 'WEB Tier 01', score: 1700, userOverride: false }]
};

describe('quality helpers', () => {
	it('finds the profile built from a preset', () => {
		expect(presetProfile([P720, P1080], QualityPreset.P1080)).toBe(P1080);
		expect(presetProfile([P720], QualityPreset.P2160)).toBeUndefined();
	});

	it('checks settings ranges', () => {
		expect(settingsDraftError(SETTINGS)).toBeUndefined();
		expect(settingsDraftError({ ...SETTINGS, defaultDelayMinutes: 1441 })).toMatch(/Delay/);
		expect(settingsDraftError({ ...SETTINGS, settleWindowSeconds: null })).toMatch(/Settle/);
		expect(settingsDraftError({ ...SETTINGS, defaultProfileId: '' })).toMatch(/default/);
	});

	it('round-trips a profile into a request with group names and quality IDs', () => {
		const request = profileRequest(profileDraft(PROFILE));
		expect(request.items[0]).toMatchObject({ groupName: '1080p', qualityIds: [3, 7] });
		expect(request.items[1].groupName).toBeUndefined();
		expect(request.formatScores).toEqual([{ formatId: 'f1', score: 1700 }]);
	});

	it('rejects a disallowed cutoff and unordered sizes', () => {
		const draft = profileDraft(PROFILE);
		expect(profileDraftError(draft)).toBeUndefined();
		draft.cutoff = 1;
		expect(profileDraftError(draft)).toMatch(/cutoff/);
		draft.cutoff = 0;
		draft.items[0].minMbPerMin = 3000;
		expect(profileDraftError(draft)).toMatch(/min ≤ preferred ≤ max/);
	});

	it('words the last sync time', () => {
		const now = new Date('2026-10-11T12:00:00Z');
		expect(formatSyncedAt(undefined, now)).toBe('Never synced');
		expect(formatSyncedAt('2026-10-11T11:30:00Z', now)).toBe('Synced 30 min ago');
		expect(formatSyncedAt('2026-10-11T09:00:00Z', now)).toBe('Synced 3 h ago');
	});
});

describe('QualitySettingsManager', () => {
	beforeEach(() => {
		vi.resetAllMocks();
		sdk.listProfiles.mockResolvedValue([P1080, P720]);
		sdk.getAcquisitionSettings.mockResolvedValue(SETTINGS);
		sdk.getTrashStatus.mockResolvedValue(NEVER_SYNCED);
		sdk.listCustomFormats.mockResolvedValue([]);
	});

	it('loads profiles, settings, TRaSH status and formats', async () => {
		const manager = new QualitySettingsManager();
		await manager.load();
		expect(manager.status).toBe(LoadStatus.READY);
		expect(manager.defaultProfileId).toBe('p1080');
		expect(manager.presetProfile(QualityPreset.P2160)).toBeUndefined();
	});

	it('shows an error when loading fails', async () => {
		sdk.listProfiles.mockRejectedValue({ data: { detail: 'boom' } });
		const manager = new QualitySettingsManager();
		await manager.load();
		expect(manager.status).toBe(LoadStatus.ERROR);
		expect(manager.error).toBe('boom');
	});

	it('makes a profile the default through the settings endpoint', async () => {
		sdk.updateAcquisitionSettings.mockImplementation(async (request) => request);
		const manager = new QualitySettingsManager();
		await manager.load();
		await manager.makeDefault('p720');
		expect(sdk.updateAcquisitionSettings).toHaveBeenCalledWith({
			...SETTINGS,
			defaultProfileId: 'p720'
		});
		expect(manager.defaultProfileId).toBe('p720');
	});

	it('waits for a newer sync attempt after Sync now', async () => {
		const synced = { enabled: true, lastAttemptAt: 'now', lastSyncedAt: 'now' };
		sdk.syncTrash.mockResolvedValue(NEVER_SYNCED);
		sdk.getTrashStatus.mockResolvedValueOnce(NEVER_SYNCED).mockResolvedValue(synced);
		const naps: number[] = [];
		const notify = vi.fn();
		const manager = new QualitySettingsManager(notify, async (ms) => {
			naps.push(ms);
		});

		await manager.syncNow();

		expect(manager.trash).toEqual(synced);
		expect(manager.syncing).toBe(false);
		expect(naps).toEqual([SYNC_POLL_MS, SYNC_POLL_MS]);
		expect(notify).toHaveBeenCalledWith('TRaSH Guides synced');
	});

	it('reports a failed sync', async () => {
		sdk.syncTrash.mockResolvedValue(NEVER_SYNCED);
		sdk.getTrashStatus.mockResolvedValue({
			enabled: true,
			lastAttemptAt: 'now',
			lastError: 'down'
		});
		const manager = new QualitySettingsManager(undefined, async () => {});
		await manager.syncNow();
		expect(manager.trashError).toBe('down');
	});

	it('blocks a term and clears the field', async () => {
		sdk.blockTerm.mockResolvedValue({ id: 'b', name: 'Block: CAM', specs: [] });
		const manager = new QualitySettingsManager();
		manager.term = '  CAM ';
		await manager.addBlockTerm();
		expect(sdk.blockTerm).toHaveBeenCalledWith({ term: 'CAM' });
		expect(manager.term).toBe('');

		await manager.addBlockTerm();
		expect(manager.blockError).toMatch(/Type/);
	});

	it('opens a profile in the editor and saves it', async () => {
		sdk.getProfile.mockResolvedValue(PROFILE);
		sdk.updateProfile.mockResolvedValue({ ...PROFILE, name: 'Full HD' });
		const manager = new QualitySettingsManager();
		await manager.openProfile('p1080');

		const editor = manager.editor!;
		editor.draft.name = 'Full HD';
		expect(await editor.save()).toBe(true);
		expect(sdk.updateProfile).toHaveBeenCalledWith(
			'p1080',
			expect.objectContaining({ name: 'Full HD' })
		);
		expect(sdk.listProfiles).toHaveBeenCalled();

		editor.setCutoff(1);
		expect(await editor.save()).toBe(false);
		expect(editor.error).toMatch(/cutoff/);
	});
});
