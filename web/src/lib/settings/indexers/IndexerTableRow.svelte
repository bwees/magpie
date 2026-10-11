<script lang="ts">
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import { Badge } from '$lib/components/ui/badge';
	import { Input } from '$lib/components/ui/input';
	import * as Select from '$lib/components/ui/select';
	import * as Table from '$lib/components/ui/table';
	import {
		formatResponseTime,
		formatSince,
		IndexerStatus,
		LIMITS,
		PRIVACY_LABELS,
		SEEDING_OVERRIDE_LABELS,
		SeedingOverride,
		seedingGoalsApply,
		type IndexerPrivacy
	} from './indexer-settings';
	import type { IndexerRow, IndexersManager } from './indexers-manager.svelte';

	let { row, manager, now }: { row: IndexerRow; manager: IndexersManager; now: Date } = $props();

	const failing = $derived(row.indexer.status === IndexerStatus.FAILING);
	const goalsApply = $derived(seedingGoalsApply(row.draft.seedingOverride));
	const save = () => manager.save(row);

	function setOverride(value: string) {
		row.draft.seedingOverride = value as SeedingOverride;
		save();
	}
</script>

<Table.Row>
	<Table.Cell class="min-w-32">
		<div class="flex items-center gap-2">
			<span class="font-medium">{row.indexer.name}</span>
			{#if row.saving}
				<LoaderCircle class="size-3.5 animate-spin text-muted-foreground" aria-label="Saving" />
			{/if}
		</div>
		<div class="text-xs text-muted-foreground">
			{PRIVACY_LABELS[row.indexer.privacy as IndexerPrivacy] ?? row.indexer.privacy}
		</div>
		{#if row.error}
			<div class="mt-1 text-xs text-destructive" role="alert">{row.error}</div>
		{/if}
	</Table.Cell>
	<Table.Cell class="max-w-48">
		{#if failing}
			<Badge variant="destructive">Failing</Badge>
			<div class="mt-1 truncate text-xs text-muted-foreground" title={row.indexer.lastError}>
				{row.indexer.lastError}
			</div>
		{:else}
			<Badge variant="secondary">OK</Badge>
		{/if}
	</Table.Cell>
	<Table.Cell class="text-sm whitespace-nowrap tabular-nums">
		{failing ? '—' : formatResponseTime(row.indexer.averageResponseMs)}
	</Table.Cell>
	<Table.Cell class="text-sm whitespace-nowrap text-muted-foreground">
		{formatSince(row.indexer.lastOkAt, now)}
	</Table.Cell>
	<Table.Cell>
		<Input
			type="number"
			class="h-8 w-16"
			aria-label="Priority for {row.indexer.name}"
			min={LIMITS.PRIORITY.MIN}
			max={LIMITS.PRIORITY.MAX}
			bind:value={row.draft.priority}
			onchange={save}
		/>
	</Table.Cell>
	<Table.Cell>
		<Input
			type="number"
			class="h-8 w-16"
			aria-label="Minimum seeders for {row.indexer.name}"
			min={LIMITS.MIN_SEEDERS.MIN}
			max={LIMITS.MIN_SEEDERS.MAX}
			bind:value={row.draft.minSeeders}
			onchange={save}
		/>
	</Table.Cell>
	<Table.Cell>
		<Select.Root type="single" value={row.draft.seedingOverride} onValueChange={setOverride}>
			<Select.Trigger size="sm" class="w-36" aria-label="After download for {row.indexer.name}">
				{SEEDING_OVERRIDE_LABELS[row.draft.seedingOverride]}
			</Select.Trigger>
			<Select.Content>
				{#each Object.values(SeedingOverride) as override (override)}
					<Select.Item value={override} label={SEEDING_OVERRIDE_LABELS[override]} />
				{/each}
			</Select.Content>
		</Select.Root>
	</Table.Cell>
	<Table.Cell>
		<div class="flex items-center gap-2">
			<Input
				type="number"
				class="h-8 w-20"
				placeholder="Global"
				aria-label="Seed ratio goal for {row.indexer.name}"
				min={LIMITS.SEED_RATIO.MIN}
				max={LIMITS.SEED_RATIO.MAX}
				step={LIMITS.SEED_RATIO.STEP}
				disabled={!goalsApply}
				bind:value={row.draft.seedRatio}
				onchange={save}
			/>
			<span class="text-xs text-muted-foreground">ratio</span>
			<Input
				type="number"
				class="h-8 w-20"
				placeholder="Global"
				aria-label="Seed days goal for {row.indexer.name}"
				min={LIMITS.SEED_DAYS.MIN}
				max={LIMITS.SEED_DAYS.MAX}
				disabled={!goalsApply}
				bind:value={row.draft.seedDays}
				onchange={save}
			/>
			<span class="text-xs text-muted-foreground">days</span>
		</div>
	</Table.Cell>
</Table.Row>
