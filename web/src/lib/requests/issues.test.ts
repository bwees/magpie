import { beforeEach, describe, expect, it, vi } from 'vitest';
import { toast } from 'svelte-sonner';
import type {
	IssueCommentResponse,
	IssueListResponse,
	IssueResponse,
	IssueThreadResponse,
	ReportIssueRequest
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import { FakeLive, settle } from '$lib/live/fake-live';
import { issue } from '$lib/testing/fixtures';
import type { IssuesApi } from './issues-api';
import { IssueFix } from './issue-fix.svelte';
import { IssuesTab } from './issues-tab.svelte';
import {
	IMPORTS_PATH,
	IssueAction,
	IssueEvent,
	IssueFilter,
	IssueKind,
	IssueProblem,
	IssueStatus,
	issueSubject,
	resolvedByPerson,
	waitingImportOf
} from './issues';
import { ReportIssue, WHOLE_TITLE, episodeOptions } from './report-issue.svelte';

vi.mock('svelte-sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const ADMIN = { id: 'admin', admin: true };
const ANA = { id: 'U1', admin: false };
const BOB = { id: 'U2', admin: false };
const WAITING_DOWNLOAD = 'D1';
const HTTP_CONFLICT = 409;

function problem(
	status: number,
	data: { type: string; detail?: string; instance?: string }
): Error {
	return Object.assign(new Error('problem'), { status, data });
}

const importWaiting = () =>
	problem(HTTP_CONFLICT, {
		type: IssueProblem.IMPORT_WAITING,
		detail: 'Show.S01E02.WEB-DL is waiting for its files to be mapped',
		instance: `/api/library/imports/${WAITING_DOWNLOAD}`
	});

class FakeIssues implements IssuesApi {
	rows: IssueResponse[] = [issue()];
	comments: IssueCommentResponse[] = [];
	acts: { id: string; action: IssueAction; waitingRemoved?: boolean }[] = [];
	removed: string[] = [];
	reported: ReportIssueRequest[] = [];
	actResult: (waitingRemoved?: boolean) => Promise<void> = async () => {};
	removeResult: () => Promise<void> = async () => {};

	async list(statuses: string[], { count = false } = {}): Promise<IssueListResponse> {
		const items = this.rows.filter((row) => statuses.includes(row.status));
		return { items: count ? [] : items, total: items.length };
	}

	async thread(id: string): Promise<IssueThreadResponse> {
		const found = this.rows.find((row) => row.id === id);
		if (!found) throw problem(404, { type: 'urn:magpie:problem:issue-not-found' });
		return { issue: found, comments: this.comments };
	}

	async report(body: ReportIssueRequest): Promise<IssueResponse> {
		this.reported.push(body);
		return issue({ id: 'I9' });
	}

	async comment(id: string, body: string): Promise<IssueCommentResponse> {
		const comment = { id: `C${this.comments.length}`, author: ADMIN_AUTHOR, body, createdAt: '' };
		this.comments = [...this.comments, comment];
		this.rows = this.rows.map((row) =>
			row.id === id ? { ...row, comments: row.comments + 1 } : row
		);
		return comment;
	}

	async act(id: string, action: IssueAction, waitingRemoved?: boolean): Promise<IssueResponse> {
		await this.actResult(waitingRemoved);
		this.acts.push({ id, action, waitingRemoved });
		this.#set(id, { status: IssueStatus.ACTION_PENDING, action });
		return this.rows.find((row) => row.id === id) ?? issue();
	}

	async resolve(id: string): Promise<IssueResponse> {
		this.#set(id, { status: IssueStatus.RESOLVED, resolvedBy: ANA.id });
		return issue({ id, status: IssueStatus.RESOLVED });
	}

	async removeWaitingImport(downloadId: string): Promise<unknown> {
		await this.removeResult();
		this.removed.push(downloadId);
		return undefined;
	}

	#set(id: string, change: Partial<IssueResponse>): void {
		this.rows = this.rows.map((row) => (row.id === id ? { ...row, ...change } : row));
	}
}

const ADMIN_AUTHOR = { id: ADMIN.id, displayName: 'Admin' };

beforeEach(() => {
	vi.mocked(toast.error).mockClear();
	vi.mocked(toast.success).mockClear();
});

describe('issue helpers', () => {
	it('names an issue by its title and episode', () => {
		expect(issueSubject(issue())).toBe('Severance · S01E02');
		expect(issueSubject(issue({ episode: undefined }))).toBe('Severance');
		expect(issueSubject(issue({ titleName: '', episode: undefined }))).toBe('Removed title');
	});

	it('reads the waiting download from an import-waiting problem only', () => {
		expect(waitingImportOf(importWaiting())).toEqual({
			downloadId: WAITING_DOWNLOAD,
			message: 'Show.S01E02.WEB-DL is waiting for its files to be mapped'
		});
		expect(
			waitingImportOf(problem(HTTP_CONFLICT, { type: 'urn:magpie:problem:issue-status' }))
		).toBe(undefined);
	});

	it('tells a person from the fix or a removed title', () => {
		expect(resolvedByPerson('U1')).toBe(true);
		expect(resolvedByPerson('ACTION')).toBe(false);
		expect(resolvedByPerson('TITLE_REMOVED')).toBe(false);
		expect(resolvedByPerson('')).toBe(false);
	});
});

describe('IssueFix', () => {
	it('asks for the fix and refetches', async () => {
		const api = new FakeIssues();
		const done = vi.fn(async () => {});
		const fix = new IssueFix(done, api);

		expect(await fix.run('I1', 'Severance', IssueAction.RERUN_SUBTITLES)).toBe(true);
		expect(api.acts).toEqual([
			{ id: 'I1', action: IssueAction.RERUN_SUBTITLES, waitingRemoved: undefined }
		]);
		expect(done).toHaveBeenCalledOnce();
		expect(fix.busy.size).toBe(0);
	});

	it('warns about a download waiting for mapping instead of failing', async () => {
		const api = new FakeIssues();
		api.actResult = async (waitingRemoved) => {
			if (!waitingRemoved) throw importWaiting();
		};
		const fix = new IssueFix(async () => {}, api);

		expect(await fix.run('I1', 'Severance', IssueAction.REGRAB)).toBe(false);

		expect(fix.held?.download.downloadId).toBe(WAITING_DOWNLOAD);
		expect(fix.releasePath).toBe(IMPORTS_PATH);
		expect(toast.error).not.toHaveBeenCalled();
	});

	it('removes the waiting download, then re-grabs past it', async () => {
		const api = new FakeIssues();
		api.actResult = async (waitingRemoved) => {
			if (!waitingRemoved) throw importWaiting();
		};
		const done = vi.fn(async () => {});
		const fix = new IssueFix(done, api);
		await fix.run('I1', 'Severance', IssueAction.REGRAB);

		await fix.removeAndContinue();

		expect(api.removed).toEqual([WAITING_DOWNLOAD]);
		expect(api.acts).toEqual([{ id: 'I1', action: IssueAction.REGRAB, waitingRemoved: true }]);
		expect(fix.held).toBeUndefined();
		expect(done).toHaveBeenCalledOnce();
	});

	it('continues when the import was already removed meanwhile', async () => {
		const api = new FakeIssues();
		api.actResult = async (waitingRemoved) => {
			if (!waitingRemoved) throw importWaiting();
		};
		api.removeResult = async () => {
			throw problem(HTTP_CONFLICT, { type: 'urn:magpie:problem:import-not-stopped' });
		};
		const fix = new IssueFix(async () => {}, api);
		await fix.run('I1', 'Severance', IssueAction.REGRAB);

		await fix.removeAndContinue();

		expect(api.acts).toHaveLength(1);
		expect(fix.held).toBeUndefined();
	});

	it('keeps the warning open when removing fails', async () => {
		const api = new FakeIssues();
		api.actResult = async () => {
			throw importWaiting();
		};
		api.removeResult = async () => {
			throw problem(500, { type: 'urn:magpie:problem:internal' });
		};
		const fix = new IssueFix(async () => {}, api);
		await fix.run('I1', 'Severance', IssueAction.REGRAB);

		await fix.removeAndContinue();

		expect(fix.held).toBeDefined();
		expect(toast.error).toHaveBeenCalledOnce();
		expect(fix.continuing).toBe(false);
	});
});

describe('IssuesTab', () => {
	it('lists open issues, counts them and says when there are none', async () => {
		const api = new FakeIssues();
		const tab = new IssuesTab(ADMIN, api);
		await tab.load();
		expect(tab.items).toHaveLength(1);
		expect(tab.openCount).toBe(1);

		await tab.select(IssueFilter.RESOLVED);
		expect(tab.empty).toBe(true);
		expect(tab.emptyText).toBe('No resolved issues.');
		await tab.select(IssueFilter.OPEN);
		expect(tab.emptyText).toBe('No open issues.');
	});

	it('shows an error with nothing loaded, and keeps rows when a refetch fails', async () => {
		const api = new FakeIssues();
		const list = vi.spyOn(api, 'list').mockRejectedValueOnce(new Error('down'));
		const tab = new IssuesTab(ADMIN, api);
		await tab.refresh();
		expect(tab.status).toBe(LoadStatus.ERROR);

		list.mockRestore();
		await tab.load();
		vi.spyOn(api, 'list').mockRejectedValueOnce(new Error('down'));
		await tab.refresh();
		expect(tab.status).toBe(LoadStatus.READY);
		expect(tab.items).toHaveLength(1);
	});

	it('lets admins fix open issues and requesters only talk about their own', () => {
		const open = issue();
		const pending = issue({ status: IssueStatus.ACTION_PENDING });
		const admin = new IssuesTab(ADMIN, new FakeIssues());
		const reporter = new IssuesTab(ANA, new FakeIssues());
		const other = new IssuesTab(BOB, new FakeIssues());

		expect(admin.canFix(open)).toBe(true);
		expect(admin.canFix(pending)).toBe(false);
		expect(reporter.canFix(open)).toBe(false);
		expect(reporter.canComment(open)).toBe(true);
		expect(reporter.canResolve(open)).toBe(true);
		expect(reporter.canResolve(issue({ status: IssueStatus.RESOLVED }))).toBe(false);
		expect(other.canComment(open)).toBe(false);
	});

	it('opens a thread and posts a reply', async () => {
		const api = new FakeIssues();
		const tab = new IssuesTab(ADMIN, api);
		await tab.load();
		await tab.open('I1');
		expect(tab.threadStatus).toBe(LoadStatus.READY);

		tab.draft = '  Looking into it  ';
		await tab.postComment();

		expect(tab.thread?.comments.map((c) => c.body)).toEqual(['Looking into it']);
		expect(tab.draft).toBe('');
		expect(tab.items[0].comments).toBe(1);
	});

	it('reloads the open thread after a fix', async () => {
		const api = new FakeIssues();
		const tab = new IssuesTab(ADMIN, api);
		await tab.load();
		await tab.open('I1');

		await tab.act(tab.items[0], IssueAction.REGRAB);

		expect(tab.thread?.issue.status).toBe(IssueStatus.ACTION_PENDING);
		expect(tab.items[0].status).toBe(IssueStatus.ACTION_PENDING);
	});

	it('refetches on issue events until stopped', async () => {
		const api = new FakeIssues();
		const tab = new IssuesTab(ANA, api);
		const live = new FakeLive();
		await tab.load();
		const stop = tab.watch(live);

		api.rows = [...api.rows, issue({ id: 'I2' })];
		live.emit(IssueEvent.REPORTED, {});
		await settle();
		expect(tab.items).toHaveLength(2);
		expect(tab.openCount).toBe(2);

		stop();
		expect(live.listeners).toBe(0);
	});
});

describe('ReportIssue', () => {
	const seasons = [
		{
			number: 1,
			monitored: true,
			episodes: [
				{ id: 'E1', season: 1, number: 1, name: 'Good News', released: true },
				{ id: 'E2', season: 1, number: 2, name: 'Later', released: false }
			].map((e) => ({
				...e,
				airDate: null,
				airedNumber: e.number,
				airedSeason: 1,
				monitored: true,
				runtimeMinutes: 50
			}))
		}
	];

	it('offers the whole series and released episodes, and nothing for a movie', () => {
		expect(episodeOptions(seasons).map((o) => o.label)).toEqual([
			'The whole series',
			'S01E01 · Good News'
		]);
		expect(episodeOptions([])).toEqual([]);
	});

	it('reports an episode with the trimmed text and closes', async () => {
		const api = new FakeIssues();
		const report = new ReportIssue('T1', api);
		report.start();
		expect(report.valid).toBe(false);

		report.kind = IssueKind.AUDIO;
		report.episodeId = 'E1';
		report.text = '  Out of sync  ';
		const reported = await report.submit();

		expect(reported?.id).toBe('I9');
		expect(api.reported).toEqual([
			{ titleId: 'T1', episodeId: 'E1', kind: IssueKind.AUDIO, text: 'Out of sync' }
		]);
		expect(report.open).toBe(false);
	});

	it('reports the whole title without an episode', async () => {
		const api = new FakeIssues();
		const report = new ReportIssue('T1', api);
		report.start();
		report.text = 'Wrong movie';
		await report.submit();
		expect(report.episodeId).toBe(WHOLE_TITLE);
		expect(api.reported[0].episodeId).toBeUndefined();
	});
});
