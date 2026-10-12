import { describe, expect, it, vi, type Mock } from 'vitest';
import type {
	CutoffStatusResponse,
	ImportResponse,
	ManualImportRequest,
	MediaFileResponse
} from '$lib/sdk';
import {
	audioTracks,
	episodeLabel,
	FileKind,
	formatDuration,
	LibraryEvent,
	MediaKind,
	StreamKind,
	videoSummary
} from './library';
import { ImportsManager, type ImportsApi, type ImportsLive } from './imports-manager.svelte';
import {
	CUTOFF_MET_EVENT,
	CutoffState,
	EPISODE_GROUP_CHANGED_EVENT,
	RENAME_SETTLE_MS,
	TitleFiles,
	type FilesApi
} from './title-files.svelte';

const E1 = { id: 'e1', season: 1, number: 1, name: 'Pilot' };
const E2 = { id: 'e2', season: 1, number: 2, name: 'Two' };

const PACK: ImportResponse = {
	download: 'd1',
	title: 't1',
	titleName: 'Show',
	kind: MediaKind.SERIES,
	releaseName: 'Show.S01.1080p-GRP',
	status: 'MANUAL',
	reason: 'Couldn’t tell which episode Extras.mkv is',
	attempts: 1,
	updatedAt: '2026-10-11T12:00:00Z',
	episodes: [E1, E2],
	files: [
		{
			path: 'Show.S01/Show.S01E01.mkv',
			size: 1,
			kind: FileKind.VIDEO,
			sample: false,
			episodes: [{ episode: 'e1', season: 1, number: 1 }]
		},
		{ path: 'Show.S01/Extras.mkv', size: 1, kind: FileKind.VIDEO, sample: false, episodes: [] },
		{ path: 'Show.S01/sample.mkv', size: 1, kind: FileKind.VIDEO, sample: true, episodes: [] },
		{
			path: 'Show.S01/Show.S01E01.en.srt',
			size: 1,
			kind: FileKind.SUBTITLE,
			sample: false,
			episodes: []
		}
	]
};

function fakeApi(
	imports: ImportResponse[] = [PACK]
): ImportsApi & { mapped: ManualImportRequest[] } {
	const mapped: ManualImportRequest[] = [];
	return {
		mapped,
		list: vi.fn(async () => imports),
		retry: vi.fn(async () => ({})),
		remove: vi.fn(async () => undefined),
		map: vi.fn(async (_download: string, request: ManualImportRequest) => {
			mapped.push(request);
			return {};
		})
	};
}

const notifier = () => ({ success: vi.fn(), error: vi.fn() });

function fakeLive(): ImportsLive & { emit: (type: string) => void } {
	const handlers = new Map<string, (m: { data: unknown }) => void>();
	return {
		on: (type, handler) => {
			handlers.set(type, handler as (m: { data: unknown }) => void);
			return () => handlers.delete(type);
		},
		onReset: () => () => undefined,
		emit: (type) => handlers.get(type)?.({ data: {} })
	};
}

describe('track summaries', () => {
	const file = {
		mediaInfo: {
			height: 2160,
			hdr: 'HDR10',
			durationSeconds: 9300,
			streams: [
				{ kind: StreamKind.VIDEO, codec: 'hevc', bitDepth: 10, language: 'und' },
				{ kind: StreamKind.AUDIO, codec: 'eac3', channels: 6, language: 'de' },
				{ kind: StreamKind.SUBTITLE, codec: 'subrip', language: 'en', forced: true, image: false }
			]
		}
	} as unknown as MediaFileResponse;

	it('describes video and audio tracks', () => {
		expect(videoSummary(file)).toBe('HEVC 2160p · 10-bit · HDR10');
		expect(audioTracks(file)).toEqual(['DE · EAC3 5.1']);
		expect(formatDuration(file.mediaInfo.durationSeconds)).toBe('2h 35m');
	});

	it('labels single and multi-episode files', () => {
		expect(episodeLabel([{ episode: 'e1', season: 1, number: 2 }])).toBe('S01E02');
		expect(
			episodeLabel([
				{ episode: 'e1', season: 1, number: 2 },
				{ episode: 'e2', season: 1, number: 3 }
			])
		).toBe('S01E02–E03');
	});
});

describe('ImportsManager', () => {
	it('opens the picker with the matched videos ticked and samples left out', async () => {
		const manager = new ImportsManager(fakeApi(), notifier());
		await manager.load();

		manager.openMapping(PACK);

		expect(manager.choices).toEqual([
			{ path: 'Show.S01/Show.S01E01.mkv', included: true, episodes: ['e1'] },
			{ path: 'Show.S01/Extras.mkv', included: true, episodes: [] },
			{ path: 'Show.S01/sample.mkv', included: false, episodes: [] }
		]);
		expect(manager.mappingProblem).toContain('Extras.mkv');
	});

	it('refuses two videos for one episode, then sends the mapping', async () => {
		const api = fakeApi();
		const manager = new ImportsManager(api, notifier());
		await manager.load();
		manager.openMapping(PACK);
		manager.choices[1].episodes = ['e1'];

		await manager.submitMapping();
		expect(manager.mappingError).toContain('same episode');
		expect(api.mapped).toHaveLength(0);

		manager.choices[1].episodes = ['e2'];
		await manager.submitMapping();
		expect(api.mapped).toEqual([
			{
				files: [
					{ path: 'Show.S01/Show.S01E01.mkv', episodes: ['e1'] },
					{ path: 'Show.S01/Extras.mkv', episodes: ['e2'] }
				]
			}
		]);
		expect(manager.mapping).toBeUndefined();
	});

	it('takes one video for a movie', async () => {
		const movie: ImportResponse = { ...PACK, kind: MediaKind.MOVIE, episodes: [] };
		const api = fakeApi([movie]);
		const manager = new ImportsManager(api, notifier());
		manager.openMapping(movie);

		manager.toggleVideo('Show.S01/Extras.mkv', true);
		await manager.submitMapping();

		expect(api.mapped).toEqual([{ files: [{ path: 'Show.S01/Extras.mkv', episodes: [] }] }]);
	});

	it('drops a retried import from the list', async () => {
		const manager = new ImportsManager(fakeApi(), notifier());
		await manager.load();
		expect(manager.count).toBe(1);

		await manager.retry(PACK);

		expect(manager.count).toBe(0);
	});

	it('asks before Remove & blocklist and sends nothing when cancelled', async () => {
		const api = fakeApi();
		const manager = new ImportsManager(api, notifier());
		await manager.load();

		manager.askRemove(PACK);
		expect(manager.confirmingRemove).toEqual(PACK);
		manager.cancelRemove();
		expect(manager.confirmingRemove).toBeUndefined();
		await manager.confirmRemove();

		expect(api.remove).not.toHaveBeenCalled();
		expect(manager.count).toBe(1);
	});

	it('removes and blocklists the import once confirmed', async () => {
		const api = fakeApi();
		const notify = notifier();
		const manager = new ImportsManager(api, notify);
		await manager.load();

		manager.askRemove(PACK);
		await manager.confirmRemove();

		expect(api.remove).toHaveBeenCalledWith(PACK.download);
		expect(manager.confirmingRemove).toBeUndefined();
		expect(manager.count).toBe(0);
		expect(notify.success).toHaveBeenCalledWith('Import removed and blocklisted', {
			description: PACK.releaseName
		});
	});

	it('reloads when another admin removes an import', async () => {
		const api = fakeApi();
		const live = fakeLive();
		const manager = new ImportsManager(api, notifier());
		manager.watch(live);

		live.emit(LibraryEvent.IMPORT_REMOVED);

		expect(api.list).toHaveBeenCalled();
	});
});

describe('TitleFiles', () => {
	const mediaFile = (id: string) => ({ id, path: `/tv/${id}.mkv` }) as MediaFileResponse;
	const status = (file: string, cutoffMet: boolean, upgradesAllowed: boolean) =>
		({ file, cutoffMet, upgradesAllowed }) as CutoffStatusResponse;

	function filesApi(
		files: MediaFileResponse[] = [],
		statuses: CutoffStatusResponse[] = []
	): FilesApi & { list: Mock; remove: Mock } {
		return {
			list: vi.fn(async () => files),
			cutoff: vi.fn(async () => statuses),
			remove: vi.fn(async () => undefined)
		};
	}

	function watched(files: TitleFiles) {
		const handlers = new Map<string, (m: { data: unknown }) => void>();
		const stop = files.watch({
			on: (type, handler) => {
				handlers.set(type, handler as (m: { data: unknown }) => void);
				return () => handlers.delete(type);
			},
			onReset: () => () => undefined
		});
		const emit = (type: string, data: unknown) => handlers.get(type)?.({ data });
		return { emit, stop };
	}

	it('reloads when a file of its title is imported, deleted, confirmed or meets the cutoff', async () => {
		const api = filesApi();
		const { emit, stop } = watched(new TitleFiles('t1', api, notifier()));

		for (const type of [
			LibraryEvent.FILE_IMPORTED,
			LibraryEvent.ITEM_AVAILABLE,
			LibraryEvent.FILE_DELETED,
			CUTOFF_MET_EVENT
		]) {
			emit(type, { title: 'other' });
			emit(type, { title: 't1' });
		}

		expect(api.list).toHaveBeenCalledTimes(4);
		stop();
	});

	it('reloads when one of its files is transcoded', async () => {
		const api = filesApi([mediaFile('f1')]);
		const files = new TitleFiles('t1', api, notifier());
		await files.load();
		const { emit, stop } = watched(files);

		emit(LibraryEvent.FILE_TRANSCODED, { mediaFile: 'elsewhere' });
		emit(LibraryEvent.FILE_TRANSCODED, { mediaFile: 'f1' });

		expect(api.list).toHaveBeenCalledTimes(2);
		stop();
	});

	it('reloads a moment after its series is renumbered, once Library has renamed the files', async () => {
		vi.useFakeTimers();
		const api = filesApi();
		const { emit, stop } = watched(new TitleFiles('t1', api, notifier()));

		emit(EPISODE_GROUP_CHANGED_EVENT, { title: 'other' });
		emit(EPISODE_GROUP_CHANGED_EVENT, { title: 't1' });
		expect(api.list).not.toHaveBeenCalled();
		await vi.advanceTimersByTimeAsync(RENAME_SETTLE_MS);

		expect(api.list).toHaveBeenCalledTimes(1);
		stop();
		vi.useRealTimers();
	});

	it('tells met, upgrading, kept and unscored files apart', async () => {
		const list = [mediaFile('met'), mediaFile('below'), mediaFile('kept'), mediaFile('new')];
		const files = new TitleFiles(
			't1',
			filesApi(list, [
				status('met', true, true),
				status('below', false, true),
				status('kept', false, false)
			]),
			notifier()
		);

		await files.load();

		expect(list.map((f) => files.cutoffOf(f))).toEqual([
			CutoffState.MET,
			CutoffState.UPGRADING,
			CutoffState.KEPT,
			CutoffState.UNKNOWN
		]);
	});

	it('deletes a file only once the delete is confirmed', async () => {
		const api = filesApi([mediaFile('f1'), mediaFile('f2')]);
		const notify = notifier();
		const files = new TitleFiles('t1', api, notify);
		await files.load();

		files.askDelete(files.files[0]);
		files.cancelDelete();
		expect(api.remove).not.toHaveBeenCalled();

		files.askDelete(files.files[0]);
		await files.confirmDelete();

		expect(api.remove).toHaveBeenCalledWith('f1');
		expect(files.files.map((f) => f.id)).toEqual(['f2']);
		expect(notify.success).toHaveBeenCalled();
	});

	it('keeps a file whose delete failed and says why', async () => {
		const api = filesApi([mediaFile('f1')]);
		api.remove.mockRejectedValue(new Error('disk is read-only'));
		const notify = notifier();
		const files = new TitleFiles('t1', api, notify);
		await files.load();

		files.askDelete(files.files[0]);
		await files.confirmDelete();

		expect(files.files).toHaveLength(1);
		expect(notify.error).toHaveBeenCalled();
		expect(files.deleting.size).toBe(0);
	});
});
