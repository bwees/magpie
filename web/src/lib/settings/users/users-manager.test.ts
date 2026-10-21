import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { UserResponse } from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import { Role } from '$lib/session/session.svelte';
import { UsersManager } from './users-manager.svelte';

const sdk = vi.hoisted(() => ({
	listUsers: vi.fn(),
	getUserDefaults: vi.fn(),
	getSignInSettings: vi.fn(),
	updateUser: vi.fn(),
	setUserDefaults: vi.fn(),
	setSignInSettings: vi.fn()
}));

vi.mock('$lib/sdk', () => ({
	...sdk,
	apiErrorMessage: (error: { data?: { detail?: string } }, fallback = 'failed') =>
		error?.data?.detail ?? fallback
}));

const NO_PERMISSIONS = {
	autoApproveMovies: false,
	autoApproveTv: false,
	viewOthersRequests: false,
	reportIssues: true,
	manageFiles: false
};
const ADMIN: UserResponse = {
	id: 'admin',
	username: 'admin',
	displayName: 'admin',
	role: Role.ADMIN,
	permissions: { ...NO_PERMISSIONS, autoApproveMovies: true },
	source: 'LOCAL',
	localAdmin: true,
	disabled: false,
	createdAt: '2026-10-11T12:00:00Z'
};
const ALICE: UserResponse = {
	...ADMIN,
	id: 'alice',
	username: 'alice',
	displayName: 'Alice',
	role: Role.REQUESTER,
	permissions: NO_PERMISSIONS,
	source: 'JELLYFIN',
	localAdmin: false
};

describe('UsersManager', () => {
	let manager: UsersManager;

	beforeEach(async () => {
		vi.resetAllMocks();
		sdk.listUsers.mockResolvedValue([ADMIN, ALICE]);
		sdk.getUserDefaults.mockResolvedValue(NO_PERMISSIONS);
		sdk.getSignInSettings.mockResolvedValue({ allowJellyfin: true });
		manager = new UsersManager(ADMIN.id);
		await manager.load();
	});

	it('loads users, defaults and the sign-in setting', () => {
		expect(manager.status).toBe(LoadStatus.READY);
		expect(manager.rows.map((row) => row.user.username)).toEqual(['admin', 'alice']);
		expect(manager.defaults).toEqual(NO_PERMISSIONS);
		expect(manager.allowJellyfin).toBe(true);
	});

	it('locks the local admin and your own row with a reason', () => {
		const [admin, alice] = manager.rows;
		expect(admin.lockedReason).toMatch(/local admin/);
		expect(alice.lockedReason).toBeUndefined();

		const self = new UsersManager(ALICE.id);
		sdk.listUsers.mockResolvedValue([ALICE]);
		return self.load().then(() => expect(self.rows[0].lockedReason).toMatch(/your own/));
	});

	it('saves a toggle and keeps the server answer', async () => {
		const alice = manager.rows[1];
		const saved = { ...ALICE, permissions: { ...NO_PERMISSIONS, autoApproveTv: true } };
		sdk.updateUser.mockResolvedValue(saved);

		await manager.setPermission(alice, 'autoApproveTv', true);

		expect(sdk.updateUser).toHaveBeenCalledWith('alice', { autoApproveTv: true });
		expect(alice.user.permissions.autoApproveTv).toBe(true);
		expect(alice.error).toBeUndefined();
	});

	it('puts the row back and shows the error inline when the server refuses', async () => {
		const alice = manager.rows[1];
		sdk.updateUser.mockRejectedValue({
			data: { detail: 'at least one enabled admin must remain' }
		});

		await manager.setRole(alice, Role.ADMIN);

		expect(alice.user.role).toBe(Role.REQUESTER);
		expect(alice.error).toBe('at least one enabled admin must remain');
	});

	it('disables a user through the enabled switch', async () => {
		const alice = manager.rows[1];
		sdk.updateUser.mockResolvedValue({ ...ALICE, disabled: true });

		await manager.setEnabled(alice, false);

		expect(sdk.updateUser).toHaveBeenCalledWith('alice', { disabled: true });
		expect(alice.user.disabled).toBe(true);
	});

	it('reverts defaults and Jellyfin sign-in on failure', async () => {
		sdk.setUserDefaults.mockRejectedValue({ data: { detail: 'nope' } });
		await manager.setDefault('autoApproveMovies', true);
		expect(manager.defaults?.autoApproveMovies).toBe(false);
		expect(manager.defaultsError).toBe('nope');

		sdk.setSignInSettings.mockRejectedValue({ data: { detail: 'nope' } });
		await manager.setAllowJellyfin(false);
		expect(manager.allowJellyfin).toBe(true);
		expect(manager.signInError).toBe('nope');
	});
});
