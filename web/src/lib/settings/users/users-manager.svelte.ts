import {
	apiErrorMessage,
	getSignInSettings,
	getUserDefaults,
	listUsers,
	setSignInSettings,
	setUserDefaults,
	updateUser,
	type Permissions,
	type UpdateUserRequest,
	type UserResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import { Role } from '$lib/session/session.svelte';

export type PermissionKey = keyof Permissions;

export const PERMISSION_TOGGLES: { key: PermissionKey; label: string }[] = [
	{ key: 'autoApproveMovies', label: 'Auto-approve movies' },
	{ key: 'autoApproveTv', label: 'Auto-approve TV' },
	{ key: 'viewOthersRequests', label: "View others' requests" },
	{ key: 'reportIssues', label: 'Report issues' }
];

/** Switch columns are narrow and centred. */
export const USER_TABLE_COLUMNS: { label: string; isSwitch: boolean }[] = [
	{ label: 'User', isSwitch: false },
	{ label: 'Role', isSwitch: false },
	...PERMISSION_TOGGLES.map((toggle) => ({ label: toggle.label, isSwitch: true })),
	{ label: 'Enabled', isSwitch: true }
];

export const ROLE_LABELS: Record<Role, string> = {
	[Role.ADMIN]: 'Admin',
	[Role.REQUESTER]: 'Requester'
};

/** Mirrors the Go `api.UserSource*` values. */
export enum UserSource {
	LOCAL = 'LOCAL',
	JELLYFIN = 'JELLYFIN'
}

export const SOURCE_LABELS: Record<UserSource, string> = {
	[UserSource.LOCAL]: 'Local admin',
	[UserSource.JELLYFIN]: 'Jellyfin'
};

const LOCAL_ADMIN_LOCKED = "The local admin can't be changed.";
const OWN_ACCOUNT_LOCKED = "You can't change your own role or permissions.";

/** One user as the table shows them, with the inline error of their last change. */
export class UserRow {
	user = $state() as UserResponse;
	error = $state<string>();
	saving = $state(false);
	readonly isSelf: boolean;

	constructor(user: UserResponse, selfId: string) {
		this.user = user;
		this.isSelf = user.id === selfId;
	}

	get isAdmin(): boolean {
		return this.user.role === Role.ADMIN;
	}

	/** Why the row can't be edited, or undefined when it can. */
	get lockedReason(): string | undefined {
		if (this.user.localAdmin) return LOCAL_ADMIN_LOCKED;
		if (this.isSelf) return OWN_ACCOUNT_LOCKED;
		return undefined;
	}
}

/** State for Settings → Users: the user table, the defaults for new users, and Jellyfin sign-in. */
export class UsersManager {
	status = $state(LoadStatus.LOADING);
	loadError = $state('');
	rows = $state<UserRow[]>([]);
	defaults = $state<Permissions>();
	defaultsError = $state<string>();
	allowJellyfin = $state(true);
	signInError = $state<string>();

	readonly #selfId: string;

	constructor(selfId: string) {
		this.#selfId = selfId;
	}

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		try {
			const [users, defaults, signIn] = await Promise.all([
				listUsers(),
				getUserDefaults(),
				getSignInSettings()
			]);
			this.rows = users.map((user) => new UserRow(user, this.#selfId));
			this.defaults = defaults;
			this.allowJellyfin = signIn.allowJellyfin;
			this.status = LoadStatus.READY;
		} catch (error) {
			this.loadError = apiErrorMessage(error, 'Could not load users.');
			this.status = LoadStatus.ERROR;
		}
	}

	setRole(row: UserRow, role: Role): Promise<void> {
		return this.#update(row, { role }, (user) => (user.role = role));
	}

	setPermission(row: UserRow, key: PermissionKey, value: boolean): Promise<void> {
		return this.#update(row, { [key]: value }, (user) => (user.permissions[key] = value));
	}

	setEnabled(row: UserRow, enabled: boolean): Promise<void> {
		return this.#update(row, { disabled: !enabled }, (user) => (user.disabled = !enabled));
	}

	async setDefault(key: PermissionKey, value: boolean): Promise<void> {
		if (!this.defaults) return;
		const previous = { ...this.defaults };
		this.defaults[key] = value;
		this.defaultsError = undefined;
		try {
			this.defaults = await setUserDefaults(this.defaults);
		} catch (error) {
			this.defaults = previous;
			this.defaultsError = apiErrorMessage(error);
		}
	}

	async setAllowJellyfin(allow: boolean): Promise<void> {
		this.allowJellyfin = allow;
		this.signInError = undefined;
		try {
			this.allowJellyfin = (await setSignInSettings({ allowJellyfin: allow })).allowJellyfin;
		} catch (error) {
			this.allowJellyfin = !allow;
			this.signInError = apiErrorMessage(error);
		}
	}

	/** Shows the change at once and puts the row back if the server refuses it. */
	async #update(
		row: UserRow,
		change: UpdateUserRequest,
		apply: (user: UserResponse) => void
	): Promise<void> {
		// Server-side Svelte (and so the unit tests) snapshots without copying.
		const previous = structuredClone($state.snapshot(row.user));
		apply(row.user);
		row.error = undefined;
		row.saving = true;
		try {
			row.user = await updateUser(row.user.id, change);
		} catch (error) {
			row.user = previous;
			row.error = apiErrorMessage(error);
		} finally {
			row.saving = false;
		}
	}
}
