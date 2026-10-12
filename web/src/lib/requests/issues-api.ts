import {
	commentIssue,
	getIssue,
	listIssues,
	removeImport,
	reportIssue,
	requestIssueAction,
	resolveIssue,
	type IssueCommentResponse,
	type IssueListResponse,
	type IssueResponse,
	type IssueThreadResponse,
	type ReportIssueRequest
} from '$lib/sdk';
import type { IssueAction, IssueStatus } from './issues';

/** Where the issue views read and act. The SDK in the app, a fake in tests. */
export interface IssuesApi {
	list(statuses: IssueStatus[], options?: { count?: boolean }): Promise<IssueListResponse>;
	thread(id: string): Promise<IssueThreadResponse>;
	report(issue: ReportIssueRequest): Promise<IssueResponse>;
	comment(id: string, body: string): Promise<IssueCommentResponse>;
	act(id: string, action: IssueAction, waitingRemoved?: boolean): Promise<IssueResponse>;
	resolve(id: string): Promise<IssueResponse>;
	/** Library's Remove & blocklist for an import waiting for its files to be mapped. */
	removeWaitingImport(downloadId: string): Promise<unknown>;
}

export const SDK_ISSUES_API: IssuesApi = {
	list: (status, { count } = {}) => listIssues({ status, count }),
	thread: (id) => getIssue(id),
	report: (issue) => reportIssue(issue),
	comment: (id, body) => commentIssue(id, { body }),
	act: (id, action, waitingRemoved) => requestIssueAction(id, { action, waitingRemoved }),
	resolve: (id) => resolveIssue(id),
	removeWaitingImport: (downloadId) => removeImport(downloadId)
};
