<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { LoadStatus } from '$lib/load-status';
	import { cn } from '$lib/utils';

	interface BlockLike {
		status: LoadStatus;
		error: string;
		empty: boolean;
	}

	let {
		block,
		label,
		empty,
		onRetry,
		skeleton,
		children,
		class: className
	}: {
		block: BlockLike;
		/** Names the block for screen readers while it loads. */
		label: string;
		/** The one-sentence empty state. */
		empty: string;
		onRetry: () => void;
		skeleton: Snippet;
		children: Snippet;
		class?: string;
	} = $props();
</script>

{#if block.status === LoadStatus.LOADING}
	<div class={className} aria-busy="true" aria-label="Loading {label}">
		{@render skeleton()}
	</div>
{:else if block.status === LoadStatus.ERROR}
	<div
		class={cn(
			'flex flex-wrap items-center justify-between gap-3 rounded-xl border border-destructive/40 bg-destructive/5 px-4 py-3 text-sm',
			className
		)}
		role="alert"
	>
		<span class="text-destructive">{block.error}</span>
		<Button size="sm" variant="outline" onclick={onRetry}>Retry</Button>
	</div>
{:else if block.empty}
	<p
		class={cn(
			'rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground',
			className
		)}
	>
		{empty}
	</p>
{:else}
	{@render children()}
{/if}
