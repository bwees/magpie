import { describe, expect, it, vi } from 'vitest';
import type {
	CandidateResponse,
	CoverageResponse,
	FileSubtitlesResponse,
	SubtitleResponse,
	SubtitleSettingsBody
} from '$lib/sdk';
import type { LiveMessage } from '$lib/live/protocol';
import { LoadStatus } from '$lib/load-status';
import { FileSubtitles, type FileSubtitlesApi } from './file-subtitles.svelte';
import { SubtitleSettings, type SubtitleSettingsApi } from './subtitle-settings.svelte';
import {
	CoverageStatus,
	coverageLabel,
	MissingReason,
	quotaLabel,
	settingsError,
	SubtitleSource,
	SubtitlesEvent,
	syncLabel,
	trackLabel,
	wantedLanguages
} from './subtitles';

const FILE = 'f1';

const PROVIDER: SubtitleResponse = {
	id: 's1',
	language: 'en',
	forced: false,
	sdh: false,
	source: SubtitleSource.PROVIDER,
	score: 92,
	synced: true,
	syncOffsetMs: -1250,
	path: '/tv/Show/Show S01E02.en.srt',
	releaseName: 'Show.S01E02.1080p-GRP',
	addedAt: '2026-10-11T12:00:00Z'
};

const COVERAGE: CoverageResponse[] = [
	{
		language: 'en',
		forced: false,
		status: CoverageStatus.SATISFIED,
		source: SubtitleSource.PROVIDER,
		waitingForQuota: false
	},
	{
		language: 'es',
		forced: false,
		status: CoverageStatus.MISSING,
		reason: MissingReason.NO_MATCH,
		waitingForQuota: false
	},
	{ language: 'es', forced: true, status: CoverageStatus.PENDING, waitingForQuota: true }
];

const STATE: FileSubtitlesResponse = { mediaFile: FILE, subtitles: [PROVIDER], coverage: COVERAGE };

const CANDIDATE: CandidateResponse = {
	fileId: 42,
	language: 'es',
	releaseName: 'Show.S01E02.1080p-GRP',
	score: 100,
	points: 360,
	maxPoints: 360,
	hashMatch: true,
	hearingImpaired: false,
	forced: false,
	aiTranslated: false,
	acceptable: true
};

const notify = () => ({ success: vi.fn(), error: vi.fn() });

function fakeApi(): FileSubtitlesApi {
	return {
		get: vi.fn(async () => STATE),
		search: vi.fn(async () => STATE),
		candidates: vi.fn(async () => [CANDIDATE]),
		download: vi.fn(async () => STATE),
		remove: vi.fn(async () => undefined)
	};
}

describe('subtitle labels', () => {
	it('names tracks, sync offsets and coverage', () => {
		expect(trackLabel({ language: 'en', forced: true, sdh: false })).toBe('English · forced');
		expect(trackLabel({ language: 'es', forced: false, sdh: true })).toBe('Spanish · SDH');
		expect(syncLabel(PROVIDER)).toBe('synced −1.3 s');
		expect(syncLabel({ ...PROVIDER, synced: false })).toBe('not synced');
		expect(syncLabel({ ...PROVIDER, source: SubtitleSource.EMBEDDED })).toBeUndefined();
		expect(COVERAGE.map(coverageLabel)).toEqual([
			'Found · OpenSubtitles',
			'Missing · no good match',
			'Waiting for quota'
		]);
		expect(wantedLanguages(COVERAGE)).toEqual(['en', 'es']);
	});

	it('labels the quota badge', () => {
		const base = { resetAt: null, pausedUntil: null, problem: '' };
		expect(quotaLabel({ ...base, remaining: null, vip: false })).toBeUndefined();
		expect(quotaLabel({ ...base, remaining: 17, vip: false })).toBe('17 left today');
		expect(quotaLabel({ ...base, remaining: 940, vip: true })).toBe('VIP · 940 left today');
	});
});

describe('FileSubtitles', () => {
	it('loads, opens the picker on a language and downloads a candidate', async () => {
		const api = fakeApi();
		const n = notify();
		const subs = new FileSubtitles(FILE, api, n);
		await subs.load();
		expect(subs.status).toBe(LoadStatus.READY);
		expect(subs.languages).toEqual(['en', 'es']);

		subs.openPicker('es', true);
		await vi.waitFor(() => expect(subs.candidatesStatus).toBe(LoadStatus.READY));
		expect(api.candidates).toHaveBeenCalledWith(FILE, 'es', true);
		expect(subs.candidates).toEqual([CANDIDATE]);

		await subs.download(CANDIDATE);
		expect(api.download).toHaveBeenCalledWith(FILE, 42, 'es', true);
		expect(subs.pickerOpen).toBe(false);
		expect(n.success).toHaveBeenCalledWith('Spanish · forced subtitle saved');
	});

	it('keeps the picker open and reports a failed download', async () => {
		const api = { ...fakeApi(), download: vi.fn(async () => Promise.reject(new Error('quota'))) };
		const n = notify();
		const subs = new FileSubtitles(FILE, api, n);
		await subs.load();
		subs.openPicker();
		await subs.download(CANDIDATE);
		expect(subs.pickerOpen).toBe(true);
		expect(n.error).toHaveBeenCalled();
		expect(subs.downloading).toBeUndefined();
	});

	it('shows an error state and recovers on retry', async () => {
		const api = fakeApi();
		vi.mocked(api.get).mockRejectedValueOnce(new Error('down'));
		const subs = new FileSubtitles(FILE, api, notify());
		await subs.load();
		expect(subs.status).toBe(LoadStatus.ERROR);
		await subs.load();
		expect(subs.status).toBe(LoadStatus.READY);
	});

	it('removes a subtitle and reloads, and reloads on its own live events only', async () => {
		const api = fakeApi();
		const subs = new FileSubtitles(FILE, api, notify());
		await subs.remove('s1');
		expect(api.remove).toHaveBeenCalledWith('s1');
		expect(api.get).toHaveBeenCalledTimes(1);

		const handlers = new Map<string, (m: LiveMessage) => void>();
		const stop = subs.watch({
			on: (type, handler) => {
				handlers.set(type, handler);
				return () => handlers.delete(type);
			},
			onReset: () => () => undefined
		});
		const message = (mediaFile: string) => ({ data: { mediaFile } }) as unknown as LiveMessage;
		handlers.get(SubtitlesEvent.SUBTITLE_ADDED)?.(message('other'));
		handlers.get(SubtitlesEvent.COVERAGE_SETTLED)?.(message(FILE));
		expect(api.get).toHaveBeenCalledTimes(2);
		stop();
		expect(handlers.size).toBe(0);
	});
});

describe('SubtitleSettings', () => {
	const SAVED: SubtitleSettingsBody = {
		whisperEnabled: true,
		whisperModel: 'large-v3-turbo',
		autoSync: true,
		minScoreEpisode: 90,
		minScoreMovie: 70,
		upgradeWindowDays: 7,
		upgradeCheckHours: 6
	};

	function settingsApi(): SubtitleSettingsApi {
		return {
			get: vi.fn(async () => SAVED),
			set: vi.fn(async (body: SubtitleSettingsBody) => body),
			quota: vi.fn(async () => ({
				remaining: 17,
				resetAt: null,
				vip: false,
				pausedUntil: null,
				problem: ''
			}))
		};
	}

	it('validates and saves only a changed draft', async () => {
		const api = settingsApi();
		const n = notify();
		const settings = new SubtitleSettings(api, n);
		await settings.load();
		expect(settings.dirty).toBe(false);
		expect(settings.quota?.remaining).toBe(17);

		settings.draft!.minScoreMovie = 101;
		expect(settings.dirty).toBe(true);
		await settings.save();
		expect(settings.draftError).toBe('Minimum scores must be 50–100%');
		expect(api.set).not.toHaveBeenCalled();

		settings.draft!.minScoreMovie = 80;
		await settings.save();
		expect(api.set).toHaveBeenCalledWith({ ...SAVED, minScoreMovie: 80 });
		expect(settings.dirty).toBe(false);
		expect(n.success).toHaveBeenCalledWith('Subtitle settings saved');
	});

	it('discards edits', async () => {
		const settings = new SubtitleSettings(settingsApi(), notify());
		await settings.load();
		settings.draft!.autoSync = false;
		settings.discard();
		expect(settings.dirty).toBe(false);
	});

	it('checks every range', () => {
		const draft = { ...SAVED };
		expect(settingsError(draft)).toBeUndefined();
		expect(settingsError({ ...draft, upgradeWindowDays: 31 })).toMatch(/upgrade window/);
		expect(settingsError({ ...draft, upgradeCheckHours: null })).toMatch(/upgrade check/);
	});
});
