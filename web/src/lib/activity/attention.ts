// Mirrors server/internal/activity/api: attention item kinds and actions.

export enum AttentionKind {
	REQUEST_PENDING = 'REQUEST_PENDING',
	ISSUE_OPEN = 'ISSUE_OPEN',
	PIPELINE_FAILED = 'PIPELINE_FAILED',
	AGENT_OFFLINE = 'AGENT_OFFLINE',
	SERVICE_DOWN = 'SERVICE_DOWN',
	TRANSCODE_FAILED = 'TRANSCODE_FAILED'
}

export enum AttentionAction {
	APPROVE = 'APPROVE',
	REGRAB = 'REGRAB',
	SEARCH_AGAIN = 'SEARCH_AGAIN',
	RETRY = 'RETRY',
	VIEW_AGENT = 'VIEW_AGENT',
	OPEN_TITLE = 'OPEN_TITLE',
	OPEN_SETTINGS = 'OPEN_SETTINGS'
}

/** Kinds that report something broken; the rest ask for a decision. */
const FAILURE_KINDS: string[] = [
	AttentionKind.PIPELINE_FAILED,
	AttentionKind.AGENT_OFFLINE,
	AttentionKind.SERVICE_DOWN,
	AttentionKind.TRANSCODE_FAILED
];

export function isFailure(kind: string): boolean {
	return FAILURE_KINDS.includes(kind);
}

/** What a row is about when it names no title. */
export const KIND_SUBJECTS: Record<string, string> = {
	[AttentionKind.REQUEST_PENDING]: 'New request',
	[AttentionKind.ISSUE_OPEN]: 'Issue',
	[AttentionKind.PIPELINE_FAILED]: 'Unknown title',
	[AttentionKind.AGENT_OFFLINE]: 'Transcode agent',
	[AttentionKind.SERVICE_DOWN]: 'Connections',
	[AttentionKind.TRANSCODE_FAILED]: 'Unknown title'
};
