import { describe, expect, it, vi } from 'vitest';
import { HttpError } from '@oazapfts/runtime';
import type { AddRootFolderRequest, HardlinkCheckResponse } from '$lib/sdk';
import { LinkMode, MediaKind } from '$lib/library/library';
import { RootFoldersManager, type RootFoldersApi } from './root-folders-manager.svelte';

const BAD_REQUEST = 400;
const MISSING_DETAIL = '/missing doesn’t exist or can’t be read';

const CHECK: HardlinkCheckResponse = {
	downloadPath: '/data/downloads',
	rootFolders: [{ id: 'r1', kind: MediaKind.MOVIE, path: '/data/movies', link: LinkMode.HARDLINK }]
};

function fakeApi(): RootFoldersApi & { added: AddRootFolderRequest[] } {
	const added: AddRootFolderRequest[] = [];
	return {
		added,
		check: vi.fn(async () => structuredClone(CHECK)),
		add: vi.fn(async (request: AddRootFolderRequest) => {
			if (request.path === '/missing') {
				throw new HttpError(BAD_REQUEST, { detail: MISSING_DETAIL }, new Headers());
			}
			added.push(request);
			return { id: 'r2', link: LinkMode.COPY, ...request };
		}),
		remove: vi.fn(async () => undefined)
	};
}

const notifier = () => ({ success: vi.fn(), error: vi.fn() });

describe('RootFoldersManager', () => {
	it('adds a folder of the chosen kind and reloads the check', async () => {
		const api = fakeApi();
		const manager = new RootFoldersManager(api, notifier());
		await manager.load();
		manager.newKind = MediaKind.SERIES;
		manager.newPath = ' /data/tv ';

		await manager.add();

		expect(api.added).toEqual([{ kind: MediaKind.SERIES, path: '/data/tv' }]);
		expect(manager.newPath).toBe('');
		expect(api.check).toHaveBeenCalledTimes(2);
	});

	it('refuses a relative path and shows the server’s reason for a bad one', async () => {
		const api = fakeApi();
		const manager = new RootFoldersManager(api, notifier());
		await manager.load();

		manager.newPath = 'media';
		await manager.add();
		expect(manager.addError).toContain('absolute');

		manager.newPath = '/missing';
		await manager.add();
		expect(api.added).toHaveLength(0);
		expect(manager.addError).toBe(MISSING_DETAIL);
	});

	it('removes a folder from the list', async () => {
		const manager = new RootFoldersManager(fakeApi(), notifier());
		await manager.load();

		await manager.remove(CHECK.rootFolders[0]);

		expect(manager.folders).toHaveLength(0);
	});
});
