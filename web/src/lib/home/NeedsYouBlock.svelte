<script lang="ts">
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import Inbox from '@lucide/svelte/icons/inbox';
	import BlockState from '$lib/components/BlockState.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { isFailure } from '$lib/activity/attention';
	import type { NeedsYou } from './needs-you.svelte';

	let { needsYou }: { needsYou: NeedsYou } = $props();

	const SKELETON_ROWS = 3;
	const block = $derived(needsYou.list.block);

	$effect(() => {
		if (needsYou.items.length > 0) void needsYou.resolve();
	});
</script>

<section aria-labelledby="needs-you">
	<h2 id="needs-you" class="mb-3 flex items-center gap-2 text-sm font-semibold">
		<Inbox class="size-4" /> Needs you
		{#if block.ready && needsYou.items.length > 0}
			<Badge class="tabular-nums">{needsYou.items.length}</Badge>
		{/if}
	</h2>
	<BlockState {block} label="what needs you" empty="All clear" onRetry={() => needsYou.list.load()}>
		{#snippet skeleton()}
			<div class="flex flex-col gap-2">
				{#each { length: SKELETON_ROWS }, i (i)}
					<Skeleton class="h-12 w-full rounded-lg" />
				{/each}
			</div>
		{/snippet}
		<ul class="divide-y rounded-xl border">
			{#each needsYou.items as item (item.id)}
				{@const button = needsYou.button(item)}
				<li class="flex items-center gap-3 px-3 py-2">
					{#if isFailure(item.kind)}
						<CircleAlert class="size-4 shrink-0 text-destructive" aria-label="Failure" />
					{:else}
						<span class="size-2 shrink-0 rounded-full bg-primary" aria-label="Action wanted"></span>
					{/if}
					<div class="min-w-0 flex-1">
						<div class="truncate text-sm font-medium">{needsYou.subject(item)}</div>
						<div class="truncate text-xs text-muted-foreground" title={item.text}>{item.text}</div>
					</div>
					{#if button.href}
						<Button size="sm" variant="outline" href={button.href}>{button.label}</Button>
					{:else}
						<Button
							size="sm"
							variant={button.primary ? 'default' : 'outline'}
							disabled={needsYou.busy.has(item.id)}
							onclick={() => button.run?.()}
						>
							{button.label}
						</Button>
					{/if}
				</li>
			{/each}
		</ul>
	</BlockState>
</section>
