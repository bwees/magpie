<script lang="ts">
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import * as Avatar from '$lib/components/ui/avatar';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import * as Sheet from '$lib/components/ui/sheet';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Textarea } from '$lib/components/ui/textarea';
	import { formatWhen } from '$lib/activity/format';
	import { titlePath } from '$lib/catalog/images';
	import { LoadStatus } from '$lib/load-status';
	import { initials } from './requests';
	import {
		ACTION_LABELS,
		IssueAction,
		IssueStatus,
		KIND_LABELS,
		PENDING_LABELS,
		ResolvedBy,
		resolvedByPerson
	} from './issues';
	import type { IssuesTab } from './issues-tab.svelte';

	let { tab }: { tab: IssuesTab } = $props();

	const issue = $derived(tab.thread?.issue);
	const comments = $derived(tab.thread?.comments ?? []);
	const SKELETON_ROWS = 2;

	function resolution(resolvedBy: string): string {
		if (resolvedBy === ResolvedBy.ACTION) return 'Resolved by the fix';
		if (resolvedBy === ResolvedBy.TITLE_REMOVED) return 'Resolved: the title was removed';
		return resolvedByPerson(resolvedBy) ? 'Resolved by hand' : 'Resolved';
	}
</script>

<Sheet.Root
	open={tab.openId !== undefined}
	onOpenChange={(open) => {
		if (!open) tab.close();
	}}
>
	<Sheet.Content class="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-lg">
		<Sheet.Header class="border-b p-4 pr-12">
			<Sheet.Title>{issue ? tab.subject(issue) : 'Issue'}</Sheet.Title>
			<Sheet.Description>
				{#if issue}
					{KIND_LABELS[issue.kind as keyof typeof KIND_LABELS] ?? issue.kind} issue reported by
					{issue.reporter.displayName || 'an unknown user'} · {formatWhen(issue.createdAt)}
				{:else}
					Loading the thread
				{/if}
			</Sheet.Description>
		</Sheet.Header>

		<div class="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4">
			{#if tab.threadStatus === LoadStatus.LOADING}
				<div class="flex flex-col gap-3" role="status" aria-label="Loading the thread">
					{#each { length: SKELETON_ROWS }, i (i)}
						<Skeleton class="h-16 w-full" />
					{/each}
				</div>
			{:else if tab.threadStatus === LoadStatus.ERROR || !issue}
				<p class="flex items-center gap-2 text-sm text-destructive" role="alert">
					<CircleAlert class="size-4" />
					{tab.threadError}
				</p>
			{:else}
				<div class="flex flex-wrap items-center gap-2">
					{#if issue.status === IssueStatus.ACTION_PENDING}
						<Badge variant="secondary">{PENDING_LABELS[issue.action as IssueAction]}</Badge>
					{:else if issue.status === IssueStatus.RESOLVED}
						<Badge variant="secondary">{resolution(issue.resolvedBy)}</Badge>
					{:else}
						<Badge variant="outline">Open</Badge>
					{/if}
					{#if issue.titleName}
						<a
							href={titlePath(issue.titleId)}
							class="text-xs text-muted-foreground hover:underline"
						>
							Open the title
						</a>
					{/if}
				</div>
				<p class="rounded-lg bg-muted p-3 whitespace-pre-wrap">{issue.text}</p>

				<ol class="flex flex-col gap-3" aria-label="Comments">
					{#each comments as comment (comment.id)}
						<li class="flex gap-3">
							<Avatar.Root class="size-7 shrink-0">
								<Avatar.Fallback class="bg-muted text-xs">
									{initials(comment.author.displayName || '?')}
								</Avatar.Fallback>
							</Avatar.Root>
							<div class="min-w-0 flex-1">
								<div class="text-xs text-muted-foreground">
									<span class="font-medium text-foreground">
										{comment.author.displayName || 'Unknown user'}
									</span>
									· {formatWhen(comment.createdAt)}
								</div>
								<p class="whitespace-pre-wrap">{comment.body}</p>
							</div>
						</li>
					{:else}
						<li class="text-sm text-muted-foreground">No replies yet.</li>
					{/each}
				</ol>
			{/if}
		</div>

		{#if issue && tab.threadStatus === LoadStatus.READY}
			<div class="flex flex-col gap-3 border-t p-4">
				{#if tab.canComment(issue)}
					<form
						class="flex flex-col gap-2"
						onsubmit={(event) => {
							event.preventDefault();
							void tab.postComment();
						}}
					>
						<Label for="issue-reply" class="sr-only">Reply</Label>
						<Textarea
							id="issue-reply"
							bind:value={tab.draft}
							placeholder="Write a reply"
							rows={3}
						/>
						<div class="flex flex-wrap justify-end gap-2">
							{#if tab.canResolve(issue)}
								<Button
									type="button"
									variant="outline"
									disabled={tab.resolving}
									onclick={() => tab.resolve(issue)}
								>
									Mark resolved
								</Button>
							{/if}
							<Button type="submit" disabled={tab.posting || tab.draft.trim() === ''}>Reply</Button>
						</div>
					</form>
				{/if}
				{#if tab.canFix(issue)}
					<div class="grid grid-cols-2 gap-2">
						<Button
							variant="outline"
							disabled={tab.busy(issue)}
							onclick={() => tab.act(issue, IssueAction.RERUN_SUBTITLES)}
						>
							{ACTION_LABELS[IssueAction.RERUN_SUBTITLES]}
						</Button>
						<Button disabled={tab.busy(issue)} onclick={() => tab.act(issue, IssueAction.REGRAB)}>
							{ACTION_LABELS[IssueAction.REGRAB]}
						</Button>
					</div>
				{/if}
			</div>
		{/if}
	</Sheet.Content>
</Sheet.Root>
