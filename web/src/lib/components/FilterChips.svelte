<script lang="ts" generics="T extends string">
	import { cn } from '$lib/utils';

	let {
		filters,
		selected,
		labels,
		count,
		onSelect
	}: {
		filters: T[];
		selected: T;
		labels: Record<T, string>;
		/** The chip's count; undefined shows none, e.g. while its source is unknown. */
		count?: (filter: T) => number | undefined;
		onSelect: (filter: T) => void;
	} = $props();
</script>

<div class="-mx-4 flex gap-2 overflow-x-auto px-4 md:mx-0 md:px-0" role="group" aria-label="Filter">
	{#each filters as filter (filter)}
		{@const shown = count?.(filter)}
		<button
			type="button"
			aria-pressed={selected === filter}
			class={cn(
				'shrink-0 rounded-full border px-3 py-1 text-sm whitespace-nowrap transition-colors',
				selected === filter ? 'border-primary bg-primary text-primary-foreground' : 'hover:bg-muted'
			)}
			onclick={() => onSelect(filter)}
		>
			{labels[filter]}
			{#if shown !== undefined}
				<span class="ml-1 tabular-nums opacity-70">{shown}</span>
			{/if}
		</button>
	{/each}
</div>
