import { toast } from 'svelte-sonner';
import { SvelteSet } from 'svelte/reactivity';
import { apiErrorMessage, apiErrorStatus } from '$lib/sdk';
import { SDK_ISSUES_API, type IssuesApi } from './issues-api';
import {
	ACTION_LABELS,
	IMPORTS_PATH,
	IssueAction,
	waitingImportOf,
	type WaitingImport
} from './issues';

/** Library's answer when the import was already removed or mapped meanwhile. */
const HTTP_CONFLICT = 409;

/** A REGRAB held back by a download waiting for its files to be mapped. */
export interface HeldRegrab {
	issueId: string;
	subject: string;
	download: WaitingImport;
}

/**
 * Runs an issue's fix. A REGRAB refused because a download still waits for mapping opens a warning: the
 * admin maps it in Activity → Imports, or removes it (Library's Remove & blocklist) and continues.
 */
export class IssueFix {
	held = $state<HeldRegrab>();
	continuing = $state(false);
	/** Issues with a fix in flight. */
	readonly busy = new SvelteSet<string>();

	readonly releasePath = IMPORTS_PATH;
	readonly #api: IssuesApi;
	readonly #done: () => Promise<void>;

	constructor(done: () => Promise<void>, api: IssuesApi = SDK_ISSUES_API) {
		this.#done = done;
		this.#api = api;
	}

	/** Asks for the fix and reports whether it was asked for. */
	async run(issueId: string, subject: string, action: IssueAction): Promise<boolean> {
		this.busy.add(issueId);
		try {
			await this.#api.act(issueId, action);
			toast.success(`${ACTION_LABELS[action]}: ${subject}`);
			await this.#done();
			return true;
		} catch (error) {
			const download = action === IssueAction.REGRAB ? waitingImportOf(error) : undefined;
			if (download) {
				this.held = { issueId, subject, download };
			} else {
				toast.error(apiErrorMessage(error));
			}
			return false;
		} finally {
			this.busy.delete(issueId);
		}
	}

	dismiss(): void {
		this.held = undefined;
	}

	/** Removes and blocklists the waiting download, then asks for the REGRAB again. */
	async removeAndContinue(): Promise<void> {
		const held = this.held;
		if (!held) return;
		this.continuing = true;
		try {
			await this.#removeWaiting(held.download.downloadId);
			await this.#api.act(held.issueId, IssueAction.REGRAB, true);
			toast.success(`${ACTION_LABELS[IssueAction.REGRAB]}: ${held.subject}`);
			this.held = undefined;
			await this.#done();
		} catch (error) {
			toast.error(apiErrorMessage(error));
		} finally {
			this.continuing = false;
		}
	}

	async #removeWaiting(downloadId: string): Promise<void> {
		try {
			await this.#api.removeWaitingImport(downloadId);
		} catch (error) {
			if (apiErrorStatus(error) !== HTTP_CONFLICT) throw error;
		}
	}
}
