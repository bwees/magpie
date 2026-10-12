<script lang="ts">
	import { goto } from '$app/navigation';
	import { toast } from 'svelte-sonner';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select';
	import { Textarea } from '$lib/components/ui/textarea';
	import type { SeasonResponse } from '$lib/sdk';
	import { ISSUE_KINDS, IssueKind, KIND_LABELS, issuePath } from './issues';
	import { episodeOptions, type ReportIssue } from './report-issue.svelte';

	let {
		report,
		titleName,
		seasons
	}: { report: ReportIssue; titleName: string; seasons?: SeasonResponse[] } = $props();

	const episodes = $derived(seasons ? episodeOptions(seasons) : []);
	const episodeLabel = $derived(
		episodes.find((option) => option.value === report.episodeId)?.label
	);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		const issue = await report.submit();
		if (!issue) return;
		toast.success('Issue reported', {
			action: { label: 'View', onClick: () => goto(issuePath(issue.id)) }
		});
	}
</script>

<Dialog.Root bind:open={report.open}>
	<Dialog.Content class="sm:max-w-md">
		<form class="grid gap-4" onsubmit={submit}>
			<Dialog.Header>
				<Dialog.Title>Report a problem with {titleName}</Dialog.Title>
				<Dialog.Description>An admin sees it and can reply or fix it.</Dialog.Description>
			</Dialog.Header>
			<div class="grid gap-2">
				<Label for="issue-kind">What's wrong</Label>
				<Select.Root
					type="single"
					value={report.kind}
					onValueChange={(value) => (report.kind = value as IssueKind)}
				>
					<Select.Trigger id="issue-kind" class="w-full">{KIND_LABELS[report.kind]}</Select.Trigger>
					<Select.Content>
						{#each ISSUE_KINDS as kind (kind)}
							<Select.Item value={kind} label={KIND_LABELS[kind]} />
						{/each}
					</Select.Content>
				</Select.Root>
			</div>
			{#if episodes.length > 0}
				<div class="grid gap-2">
					<Label for="issue-episode">Episode</Label>
					<Select.Root
						type="single"
						value={report.episodeId}
						onValueChange={(value) => (report.episodeId = value)}
					>
						<Select.Trigger id="issue-episode" class="w-full">
							<span class="truncate">{episodeLabel}</span>
						</Select.Trigger>
						<Select.Content class="max-h-72">
							{#each episodes as option (option.value)}
								<Select.Item value={option.value} label={option.label} />
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
			{/if}
			<div class="grid gap-2">
				<Label for="issue-text">Details</Label>
				<Textarea
					id="issue-text"
					bind:value={report.text}
					rows={4}
					placeholder="The audio drifts out of sync after 20 minutes"
				/>
			</div>
			<Dialog.Footer>
				<Button type="button" variant="outline" onclick={() => (report.open = false)}>Cancel</Button
				>
				<Button type="submit" disabled={report.submitting || !report.valid}>Report issue</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
