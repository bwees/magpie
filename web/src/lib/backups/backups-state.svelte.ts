import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	createBackup,
	listBackups,
	restoreBackup,
	type BackupResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';

/** Mirrors the Go `db.BackupKind` values. */
export enum BackupKind {
	DAILY = 'DAILY',
	PRE_MIGRATION = 'PRE_MIGRATION',
	PRE_RESTORE = 'PRE_RESTORE',
	MANUAL = 'MANUAL'
}

export const BACKUP_KIND_LABELS: Record<BackupKind, string> = {
	[BackupKind.DAILY]: 'Daily',
	[BackupKind.PRE_MIGRATION]: 'Before upgrade',
	[BackupKind.PRE_RESTORE]: 'Before restore',
	[BackupKind.MANUAL]: 'Manual'
};

const BYTES_PER_UNIT = 1024;
const SIZE_UNITS = ['B', 'KB', 'MB', 'GB'];

const ONE_DECIMAL_BELOW = 10;

export function formatSize(bytes: number): string {
	const unit =
		bytes < BYTES_PER_UNIT
			? 0
			: Math.min(Math.floor(Math.log(bytes) / Math.log(BYTES_PER_UNIT)), SIZE_UNITS.length - 1);
	const size = bytes / BYTES_PER_UNIT ** unit;
	const shown = unit > 0 && size < ONE_DECIMAL_BELOW ? size.toFixed(1) : String(Math.round(size));
	return `${shown} ${SIZE_UNITS[unit]}`;
}

/** Settings → General: the backup list, "Back up now" and Restore. */
export class BackupsState {
	status = $state(LoadStatus.LOADING);
	error = $state('');
	backups = $state<BackupResponse[]>([]);
	creating = $state(false);
	restoring = $state<string | undefined>();

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		try {
			this.backups = await listBackups();
			this.status = LoadStatus.READY;
		} catch (error) {
			this.error = apiErrorMessage(error, 'Could not load backups.');
			this.status = LoadStatus.ERROR;
		}
	}

	async create(): Promise<void> {
		this.creating = true;
		try {
			const backup = await createBackup();
			this.backups = [backup, ...this.backups.filter((b) => b.name !== backup.name)];
			toast.success('Backup created', {
				description: `${backup.name} · ${formatSize(backup.size)}`
			});
		} catch (error) {
			toast.error('Backup failed', { description: apiErrorMessage(error) });
		} finally {
			this.creating = false;
		}
	}

	/** The backup whose restore waits for confirmation. */
	confirming = $state<string | undefined>();

	askToRestore(name: string): void {
		this.confirming = name;
	}

	cancelRestore(): void {
		this.confirming = undefined;
	}

	async restoreConfirmed(): Promise<void> {
		const name = this.confirming;
		this.confirming = undefined;
		if (name) await this.#restore(name);
	}

	async #restore(name: string): Promise<void> {
		this.restoring = name;
		try {
			await restoreBackup(name);
			toast.success('Restoring backup', {
				description: 'Magpie is restarting. Reload the page in a moment.'
			});
		} catch (error) {
			toast.error('Restore failed', { description: apiErrorMessage(error) });
		} finally {
			this.restoring = undefined;
		}
	}
}
