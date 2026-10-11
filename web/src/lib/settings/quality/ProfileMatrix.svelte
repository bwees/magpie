<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select';
	import { Switch } from '$lib/components/ui/switch';
	import * as Table from '$lib/components/ui/table';
	import { PROPERS_LABELS, PropersMode } from './quality';
	import type { ProfileEditor } from './profile-editor.svelte';

	let {
		editor,
		isDefault,
		onDelete
	}: { editor: ProfileEditor; isDefault: boolean; onDelete: () => void } = $props();

	const draft = $derived(editor.draft);
	const SIZE_STEP = 0.1;
	const SIZE_COLUMNS = ['Min', 'Preferred', 'Max'];
</script>

<form
	class="grid min-w-0 gap-5"
	onsubmit={(event) => {
		event.preventDefault();
		editor.save();
	}}
>
	<div class="grid gap-4 sm:grid-cols-2">
		<div class="grid gap-1.5">
			<Label for="profile-name">Name</Label>
			<Input id="profile-name" bind:value={draft.name} />
		</div>
		<div class="grid gap-1.5">
			<Label for="propers">Propers and repacks</Label>
			<Select.Root
				type="single"
				value={draft.propersMode}
				onValueChange={(value) => (draft.propersMode = value as PropersMode)}
			>
				<Select.Trigger id="propers" class="w-full"
					>{PROPERS_LABELS[draft.propersMode]}</Select.Trigger
				>
				<Select.Content>
					{#each Object.values(PropersMode) as mode (mode)}
						<Select.Item value={mode} label={PROPERS_LABELS[mode]} />
					{/each}
				</Select.Content>
			</Select.Root>
		</div>
	</div>

	<div class="min-w-0">
		<div class="mb-2 flex items-center justify-between gap-2">
			<h4 class="text-sm font-medium">
				Qualities <span class="font-normal text-muted-foreground"
					>· most preferred first, MB per minute</span
				>
			</h4>
		</div>
		<div class="overflow-x-auto rounded-lg border">
			<Table.Root class="min-w-[36rem]">
				<Table.Header>
					<Table.Row>
						<Table.Head>Quality or group</Table.Head>
						<Table.Head>Allowed</Table.Head>
						<Table.Head>Cutoff</Table.Head>
						{#each SIZE_COLUMNS as column (column)}
							<Table.Head>{column}</Table.Head>
						{/each}
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each draft.items as item, index (item.label)}
						<Table.Row>
							<Table.Cell>
								<div class="font-medium">{item.label}</div>
								{#if item.groupName}
									<div class="text-xs text-muted-foreground">
										Group of {item.qualityIds.length}, ranked equally
									</div>
								{/if}
							</Table.Cell>
							<Table.Cell>
								<Checkbox
									checked={item.allowed}
									onCheckedChange={(checked) => editor.setAllowed(index, checked)}
									aria-label="Allow {item.label}"
								/>
							</Table.Cell>
							<Table.Cell>
								<input
									type="radio"
									name="cutoff"
									class="size-4 accent-primary"
									checked={draft.cutoff === index}
									onchange={() => editor.setCutoff(index)}
									aria-label="Cutoff at {item.label}"
								/>
							</Table.Cell>
							<Table.Cell>
								<Input
									type="number"
									class="h-8 w-20"
									min="0"
									step={SIZE_STEP}
									bind:value={item.minMbPerMin}
									aria-label="Minimum size for {item.label}"
								/>
							</Table.Cell>
							<Table.Cell>
								<Input
									type="number"
									class="h-8 w-20"
									min="0"
									step={SIZE_STEP}
									bind:value={item.preferredMbPerMin}
									aria-label="Preferred size for {item.label}"
								/>
							</Table.Cell>
							<Table.Cell>
								<Input
									type="number"
									class="h-8 w-20"
									min="0"
									step={SIZE_STEP}
									bind:value={item.maxMbPerMin}
									aria-label="Maximum size for {item.label}"
								/>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</div>
	</div>

	<div class="grid gap-4 sm:grid-cols-3">
		<div class="grid gap-1.5">
			<Label for="min-score">Minimum format score</Label>
			<Input id="min-score" type="number" bind:value={draft.minFormatScore} />
		</div>
		<div class="grid gap-1.5">
			<Label for="cutoff-score">Upgrade until score</Label>
			<Input id="cutoff-score" type="number" bind:value={draft.cutoffFormatScore} />
		</div>
		<div class="grid gap-1.5">
			<Label for="increment">Minimum upgrade step</Label>
			<Input id="increment" type="number" bind:value={draft.minUpgradeFormatScore} />
		</div>
	</div>

	<label class="flex items-center gap-3 text-sm">
		<Switch bind:checked={draft.upgradeAllowed} />
		Upgrade files until the cutoff is met
	</label>

	<div>
		<h4 class="mb-2 text-sm font-medium">Format scores</h4>
		{#if draft.formatScores.length === 0}
			<p class="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
				No custom formats are scored yet. TRaSH sync adds them, and "Block term" adds one to every
				profile.
			</p>
		{:else}
			<div class="divide-y rounded-lg border">
				{#each draft.formatScores as score (score.formatId)}
					<div class="flex flex-col gap-2 px-3 py-2 sm:flex-row sm:items-center sm:justify-between">
						<span class="flex flex-wrap items-center gap-2 text-sm">
							{score.name}
							{#if score.userOverride}
								<Badge variant="outline">Your score</Badge>
							{/if}
						</span>
						<Input
							type="number"
							class="h-8 w-28"
							bind:value={score.score}
							aria-label="Score for {score.name}"
						/>
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<div class="flex flex-wrap items-center gap-3">
		<Button type="submit" size="sm" disabled={editor.saving}>
			{editor.saving ? 'Saving…' : 'Save profile'}
		</Button>
		<Button type="button" size="sm" variant="ghost" onclick={() => editor.reset()}>Reset</Button>
		<Button
			type="button"
			size="sm"
			variant="ghost"
			class="text-destructive sm:ml-auto"
			disabled={isDefault}
			title={isDefault ? 'Pick another default profile first' : undefined}
			onclick={onDelete}
		>
			Delete profile
		</Button>
		{#if editor.error}
			<span class="w-full text-sm text-destructive" role="alert">{editor.error}</span>
		{/if}
	</div>
</form>
