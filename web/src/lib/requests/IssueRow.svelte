<script lang="ts">
	import MessageSquare from '@lucide/svelte/icons/message-square';
	import * as Avatar from '$lib/components/ui/avatar';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { formatWhen } from '$lib/activity/format';
	import type { IssueResponse } from '$lib/sdk';
	import { initials } from './requests';
	import { ACTION_LABELS, IssueAction, IssueStatus, KIND_LABELS, PENDING_LABELS } from './issues';
	import type { IssuesTab } from './issues-tab.svelte';

	let { issue, tab }: { issue: IssueResponse; tab: IssuesTab } = $props();

	const kind = $derived(KIND_LABELS[issue.kind as keyof typeof KIND_LABELS] ?? issue.kind);
	const busy = $derived(tab.busy(issue));
	const pending = $derived(
		issue.status === IssueStatus.ACTION_PENDING
			? PENDING_LABELS[issue.action as IssueAction]
			: undefined
	);
	const resolved = $derived(issue.status === IssueStatus.RESOLVED);
	const commentLabel = $derived(issue.comments === 1 ? '1 comment' : `${issue.comments} comments`);
</script>

<li class="flex flex-col gap-3 py-3 md:flex-row md:items-center md:gap-4">
	<button
		type="button"
		class="flex min-w-0 flex-1 flex-col items-start gap-1 text-left"
		onclick={() => tab.open(issue.id)}
		aria-label="Open the thread about {tab.subject(issue)}"
	>
		<div class="flex w-full min-w-0 items-center gap-2">
			<Badge variant="outline" class="shrink-0">{kind}</Badge>
			<span class="truncate font-medium">{tab.subject(issue)}</span>
		</div>
		<p class="line-clamp-2 text-sm text-muted-foreground">{issue.text}</p>
		<div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
			<Avatar.Root class="size-5">
				<Avatar.Fallback class="bg-muted text-[10px]">
					{initials(issue.reporter.displayName || '?')}
				</Avatar.Fallback>
			</Avatar.Root>
			<span class="truncate">{issue.reporter.displayName || 'Unknown user'}</span>
			<span>· {formatWhen(issue.createdAt)}</span>
			<span class="flex items-center gap-1" title={commentLabel}>
				· <MessageSquare class="size-3.5" />
				<span class="tabular-nums">{issue.comments}</span>
				<span class="sr-only">{commentLabel}</span>
			</span>
		</div>
	</button>

	{#if pending || resolved}
		<div class="shrink-0">
			<Badge variant="secondary">{pending ?? 'Resolved'}</Badge>
		</div>
	{/if}

	{#if tab.canFix(issue)}
		<div class="grid grid-cols-2 gap-2 md:flex md:shrink-0 md:justify-end">
			<Button
				size="sm"
				variant="outline"
				disabled={busy}
				onclick={() => tab.act(issue, IssueAction.RERUN_SUBTITLES)}
			>
				{ACTION_LABELS[IssueAction.RERUN_SUBTITLES]}
			</Button>
			<Button size="sm" disabled={busy} onclick={() => tab.act(issue, IssueAction.REGRAB)}>
				{ACTION_LABELS[IssueAction.REGRAB]}
			</Button>
		</div>
	{/if}
</li>
