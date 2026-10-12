import { describe, expect, it, vi } from 'vitest';
import type { BacklogResponse, UpdateTranscodeSettingsRequest } from '$lib/sdk';
import { transcodeSettings } from '$lib/testing/fixtures';
import {
	TranscodeSettingsManager,
	type TranscodeSettingsApi
} from './transcode-settings-manager.svelte';
import {
	BITRATE_CAP_MBPS,
	Preset,
	ResolutionClass,
	SkipReason,
	TargetCodec,
	toUpdate
} from './transcode-settings';

const BACKLOG: BacklogResponse = {
	backlog: 12,
	eligible: 40,
	skipped: { [SkipReason.NO_CAPABLE_AGENT]: 3 }
};

function fakeApi() {
	return {
		settings: vi.fn(async () => transcodeSettings()),
		save: vi.fn(async (body: UpdateTranscodeSettingsRequest) => transcodeSettings(body)),
		backlog: vi.fn(async () => BACKLOG),
		sweep: vi.fn(async () => BACKLOG)
	} satisfies TranscodeSettingsApi;
}

function notify() {
	return { success: vi.fn(), error: vi.fn() };
}

async function loaded(api = fakeApi(), notifier = notify()) {
	const manager = new TranscodeSettingsManager(api, notifier);
	await manager.load();
	return manager;
}

describe('TranscodeSettingsManager', () => {
	it('loads the settings and the backlog without anything to save', async () => {
		const manager = await loaded();

		expect(manager.targetCodec).toBe(TargetCodec.HEVC_10);
		expect(manager.caps[ResolutionClass.FHD]).toBe(10);
		expect(manager.backlog).toEqual(BACKLOG);
		expect(manager.dirty).toBe(false);
	});

	it('saves the whole policy and keeps the agent limits as they were', async () => {
		const api = fakeApi();
		const manager = await loaded(api);

		manager.targetCodec = TargetCodec.AV1_10;
		manager.preset = Preset.SMALL;
		manager.caps[ResolutionClass.UHD] = 30;
		manager.overrides['libsvtav1'] = 40;
		await manager.save();

		expect(api.save).toHaveBeenCalledWith({
			...toUpdate(transcodeSettings()),
			targetCodec: TargetCodec.AV1_10,
			preset: Preset.SMALL,
			bitrateCapsMbps: { ...transcodeSettings().bitrateCapsMbps, [ResolutionClass.UHD]: 30 },
			encoderOverrides: { libsvtav1: 40 }
		});
		expect(manager.dirty).toBe(false);
	});

	it('refuses out-of-range caps, percentages and overrides', async () => {
		const manager = await loaded();

		manager.caps[ResolutionClass.SD] = BITRATE_CAP_MBPS.MAX + 1;
		expect(manager.canSave).toBe(false);
		manager.caps[ResolutionClass.SD] = 2;

		manager.lowBitratePct = 5;
		expect(manager.canSave).toBe(false);
		manager.lowBitratePct = 50;

		manager.overrides['hevc_nvenc'] = 52;
		expect(manager.canSave).toBe(false);
		manager.overrides['hevc_nvenc'] = null;
		expect(manager.dirty).toBe(false);
	});

	it('shows the calibrated value of the selected preset', async () => {
		const manager = await loaded();
		const nvenc = manager.encoders[0];

		expect(manager.calibrated(nvenc)).toBe(23);
		manager.preset = Preset.ARCHIVE;
		expect(manager.calibrated(nvenc)).toBe(19);
	});

	it('discards edits', async () => {
		const manager = await loaded();
		manager.enabled = false;
		manager.minSavingPct = 30;

		manager.discard();

		expect(manager.enabled).toBe(true);
		expect(manager.minSavingPct).toBe(15);
	});

	it('re-scans the library and reports a failure', async () => {
		const api = fakeApi();
		const notifier = notify();
		const manager = await loaded(api, notifier);

		await manager.rescan();
		expect(api.sweep).toHaveBeenCalledOnce();
		expect(notifier.success).toHaveBeenCalled();

		api.sweep.mockRejectedValueOnce(new Error('down'));
		await manager.rescan();
		expect(notifier.error).toHaveBeenCalled();
		expect(manager.sweeping).toBe(false);
	});
});
