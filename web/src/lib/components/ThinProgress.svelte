<script lang="ts">
	import { cn } from '$lib/utils';

	/** percent is 0–100; undefined shows work with no known progress. */
	let { percent, class: className }: { percent?: number; class?: string } = $props();

	const FULL_PERCENT = 100;
</script>

<div
	class={cn('h-1 overflow-hidden rounded-full bg-muted', className)}
	role="progressbar"
	aria-valuemin={0}
	aria-valuemax={FULL_PERCENT}
	aria-valuenow={percent}
>
	{#if percent === undefined}
		<div class="h-full w-1/3 animate-pulse rounded-full bg-primary/60"></div>
	{:else}
		<div class="h-full rounded-full bg-primary transition-[width]" style:width="{percent}%"></div>
	{/if}
</div>
