import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	type IssueCommentResponse,
	type IssueResponse,
	type IssueThreadResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import type { LiveEvents } from '$lib/activity/activity-source';
import { SDK_ISSUES_API, type IssuesApi } from './issues-api';
import { IssueFix } from './issue-fix.svelte';
import {
	ISSUE_EVENTS,
	ISSUE_FILTER_STATUSES,
	IssueAction,
	IssueFilter,
	IssueStatus,
	issueSubject
} from './issues';
import type { Viewer } from './requests-page.svelte';

/**
 * The Issues tab: open or resolved issues the viewer may see, one issue's comment thread, replies, the
 * admin's fixes and closing an issue. Every issue event refetches.
 */
export class IssuesTab {
	filter = $state(IssueFilter.OPEN);
	status = $state(LoadStatus.LOADING);
	error = $state('');
	items = $state<IssueResponse[]>([]);
	openCount = $state(0);

	/** The issue whose thread is open. */
	openId = $state<string>();
	thread = $state<IssueThreadResponse>();
	threadStatus = $state(LoadStatus.LOADING);
	threadError = $state('');
	draft = $state('');
	posting = $state(false);
	resolving = $state(false);

	readonly viewer: Viewer;
	readonly fix: IssueFix;
	readonly #api: IssuesApi;
	#latest: Promise<unknown> | undefined;

	constructor(viewer: Viewer, api: IssuesApi = SDK_ISSUES_API) {
		this.viewer = viewer;
		this.#api = api;
		this.fix = new IssueFix(() => this.#changed(), api);
	}

	get empty(): boolean {
		return this.status === LoadStatus.READY && this.items.length === 0;
	}

	get emptyText(): string {
		return this.filter === IssueFilter.OPEN ? 'No open issues.' : 'No resolved issues.';
	}

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		await Promise.all([this.refresh(), this.loadCount()]);
	}

	select(filter: IssueFilter): Promise<void> {
		this.filter = filter;
		return this.load();
	}

	/** Refetches the rows; the newest call wins when several overlap. */
	async refresh(): Promise<void> {
		const request = this.#api.list(ISSUE_FILTER_STATUSES[this.filter]);
		this.#latest = request;
		try {
			const result = await request;
			if (this.#latest !== request) return;
			this.items = result.items;
			this.error = '';
			this.status = LoadStatus.READY;
		} catch (error) {
			if (this.#latest !== request) return;
			this.error = apiErrorMessage(error, 'Could not load issues.');
			if (this.items.length === 0) this.status = LoadStatus.ERROR;
		}
	}

	async loadCount(): Promise<void> {
		try {
			this.openCount = (
				await this.#api.list(ISSUE_FILTER_STATUSES[IssueFilter.OPEN], { count: true })
			).total;
		} catch {
			// The tab keeps its last count; the next event or reconnect tries again.
		}
	}

	async open(id: string): Promise<void> {
		this.openId = id;
		this.thread = undefined;
		this.draft = '';
		this.threadStatus = LoadStatus.LOADING;
		await this.#loadThread(id);
	}

	close(): void {
		this.openId = undefined;
		this.thread = undefined;
	}

	async #loadThread(id: string): Promise<void> {
		try {
			const thread = await this.#api.thread(id);
			if (this.openId !== id) return;
			this.thread = thread;
			this.threadError = '';
			this.threadStatus = LoadStatus.READY;
		} catch (error) {
			if (this.openId !== id) return;
			this.threadError = apiErrorMessage(error, 'Could not load this issue.');
			if (!this.thread) this.threadStatus = LoadStatus.ERROR;
		}
	}

	subject(issue: IssueResponse): string {
		return issueSubject(issue);
	}

	isReporter(issue: IssueResponse): boolean {
		return issue.reporter.id === this.viewer.id;
	}

	canFix(issue: IssueResponse): boolean {
		return this.viewer.admin && issue.status === IssueStatus.OPEN;
	}

	canComment(issue: IssueResponse): boolean {
		return this.viewer.admin || this.isReporter(issue);
	}

	canResolve(issue: IssueResponse): boolean {
		return this.canComment(issue) && issue.status !== IssueStatus.RESOLVED;
	}

	busy(issue: IssueResponse): boolean {
		return this.fix.busy.has(issue.id);
	}

	act(issue: IssueResponse, action: IssueAction): Promise<boolean> {
		return this.fix.run(issue.id, this.subject(issue), action);
	}

	async postComment(): Promise<IssueCommentResponse | undefined> {
		const id = this.openId;
		const body = this.draft.trim();
		if (!id || body === '') return undefined;
		this.posting = true;
		try {
			const comment = await this.#api.comment(id, body);
			this.draft = '';
			await Promise.all([this.#loadThread(id), this.refresh()]);
			return comment;
		} catch (error) {
			toast.error(apiErrorMessage(error));
			return undefined;
		} finally {
			this.posting = false;
		}
	}

	async resolve(issue: IssueResponse): Promise<void> {
		this.resolving = true;
		try {
			await this.#api.resolve(issue.id);
			toast.success(`Resolved: ${this.subject(issue)}`);
			await this.#changed();
		} catch (error) {
			toast.error(apiErrorMessage(error));
		} finally {
			this.resolving = false;
		}
	}

	async #changed(): Promise<void> {
		const open = this.openId;
		await Promise.all([
			this.refresh(),
			this.loadCount(),
			open ? this.#loadThread(open) : undefined
		]);
	}

	/** Follows issue events until the returned function is called. */
	watch(live: LiveEvents): () => void {
		const stops = ISSUE_EVENTS.map((type) => live.on(type, () => void this.#changed()));
		stops.push(live.onReset(() => void this.#changed()));
		return () => stops.forEach((stop) => stop());
	}
}
