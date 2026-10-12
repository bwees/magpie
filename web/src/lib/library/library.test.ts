import { describe, expect, it, vi } from 'vitest';
import type { ImportResponse, ManualImportRequest, MediaFileResponse } from '$lib/sdk';
import {
	audioTracks,
	episodeLabel,
	FileKind,
	formatDuration,
	LibraryEvent,
	MediaKind,
	StreamKind,
	subtitleTracks,
	videoSummary
} from './library';
import { ImportsManager, type ImportsApi, type ImportsLive } from './imports-manager.svelte';
import { TitleFiles } from './title-files.svelte';

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

	it('describes video, audio and subtitle tracks', () => {
		expect(videoSummary(file)).toBe('HEVC 2160p · 10-bit · HDR10');
		expect(audioTracks(file)).toEqual(['DE · EAC3 5.1']);
		expect(subtitleTracks(file)).toEqual(['EN · SUBRIP · forced']);
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
	it('reloads when a file of its title is imported', async () => {
		const api = vi.fn(async () => []);
		const files = new TitleFiles('t1', api);
		const handlers = new Map<string, (m: { data: unknown }) => void>();
		const stop = files.watch({
			on: (type, handler) => {
				handlers.set(type, handler as (m: { data: unknown }) => void);
				return () => handlers.delete(type);
			},
			onReset: () => () => undefined
		});

		handlers.get(LibraryEvent.FILE_IMPORTED)?.({ data: { title: 'other' } });
		handlers.get(LibraryEvent.FILE_IMPORTED)?.({ data: { title: 't1' } });
		handlers.get(LibraryEvent.ITEM_AVAILABLE)?.({ data: { title: 'other' } });
		handlers.get(LibraryEvent.ITEM_AVAILABLE)?.({ data: { title: 't1' } });

		expect(api).toHaveBeenCalledTimes(2);
		stop();
	});

	it('lists the files Jellyfin is overdue on', async () => {
		const file = (id: string, jellyfinOverdue: boolean) =>
			({ id, jellyfinOverdue }) as MediaFileResponse;
		const files = new TitleFiles(
			't1',
			vi.fn(async () => [file('late', true), file('fine', false)])
		);

		await files.load();

		expect(files.overdue.map((f) => f.id)).toEqual(['late']);
	});
});
