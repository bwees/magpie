<script lang="ts">
	import Search from '@lucide/svelte/icons/search';
	import { Input } from '$lib/components/ui/input';
	import { SearchLayout } from './layout';
	import SearchResults from './SearchResults.svelte';
	import type { TitleSearch } from './title-search.svelte';

	let { search }: { search: TitleSearch } = $props();

	const ESCAPE_KEY = 'Escape';

	let root: HTMLElement | undefined = $state();

	function closeOnOutsidePointer(event: PointerEvent) {
		if (search.open && root && !root.contains(event.target as Node)) {
			search.close();
		}
	}

	function closeOnEscape(event: KeyboardEvent) {
		if (search.open && event.key === ESCAPE_KEY) {
			search.close();
		}
	}
</script>

<svelte:window onpointerdown={closeOnOutsidePointer} onkeydown={closeOnEscape} />

<div bind:this={root} class="relative w-56 lg:w-80">
	<Search
		class="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
	/>
	<Input
		type="search"
		class="pl-8"
		placeholder="Search or add titles"
		aria-label="Search titles"
		aria-expanded={search.open}
		value={search.query}
		onfocus={() => search.show()}
		oninput={(event) => search.type(event.currentTarget.value)}
	/>
	{#if search.open}
		<div
			class="absolute top-11 right-0 z-50 max-h-[75vh] w-[36rem] max-w-[calc(100vw-2rem)] overflow-y-auto rounded-xl border bg-popover p-3 text-popover-foreground shadow-xl"
		>
			<SearchResults {search} layout={SearchLayout.POPOVER} />
		</div>
	{/if}
</div>
