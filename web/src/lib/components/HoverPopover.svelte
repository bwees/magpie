<script lang="ts">
	import type { Snippet } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import * as HoverCard from '$lib/components/ui/hover-card';
	import * as Popover from '$lib/components/ui/popover';

	/** Opens on hover where the device can hover, and on tap where it can't. */
	let {
		trigger,
		children,
		class: className = 'w-72',
		align = 'end'
	}: {
		trigger: Snippet<[Record<string, unknown>]>;
		children: Snippet;
		class?: string;
		align?: 'start' | 'center' | 'end';
	} = $props();

	const HOVER_DELAY_MS = 80;
	const canHover = new MediaQuery('(hover: hover)');
</script>

{#if canHover.current}
	<HoverCard.Root openDelay={HOVER_DELAY_MS} closeDelay={HOVER_DELAY_MS}>
		<HoverCard.Trigger>
			{#snippet child({ props })}{@render trigger(props)}{/snippet}
		</HoverCard.Trigger>
		<HoverCard.Content class={className} {align}>{@render children()}</HoverCard.Content>
	</HoverCard.Root>
{:else}
	<Popover.Root>
		<Popover.Trigger>
			{#snippet child({ props })}{@render trigger(props)}{/snippet}
		</Popover.Trigger>
		<Popover.Content class={className} {align}>{@render children()}</Popover.Content>
	</Popover.Root>
{/if}
