<script lang="ts">
	import Search from '@lucide/svelte/icons/search';
	import X from '@lucide/svelte/icons/x';
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { SearchLayout } from './layout';
	import SearchResults from './SearchResults.svelte';
	import type { TitleSearch } from './title-search.svelte';

	let { search }: { search: TitleSearch } = $props();
</script>

<Button
	variant="ghost"
	size="icon"
	class="rounded-full"
	aria-label="Search"
	onclick={() => search.show()}
>
	<Search class="size-5" />
</Button>

<Dialog.Root
	open={search.open}
	onOpenChange={(open) => {
		if (!open) search.close();
	}}
>
	<Dialog.Content
		showCloseButton={false}
		class="top-0 left-0 flex h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 rounded-none p-0 ring-0 sm:max-w-none"
	>
		<Dialog.Title class="sr-only">Search titles</Dialog.Title>
		<div
			class="flex items-center gap-2 border-b p-3 pt-[calc(--spacing(3)+env(safe-area-inset-top))]"
		>
			<div class="relative flex-1">
				<Search
					class="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
				/>
				<Input
					type="search"
					class="pl-8"
					placeholder="Search or add titles"
					aria-label="Search titles"
					value={search.query}
					oninput={(event) => search.type(event.currentTarget.value)}
				/>
			</div>
			<Button variant="ghost" size="icon" aria-label="Close search" onclick={() => search.close()}>
				<X class="size-5" />
			</Button>
		</div>
		<div class="flex-1 overflow-y-auto p-4">
			<SearchResults {search} layout={SearchLayout.OVERLAY} />
		</div>
	</Dialog.Content>
</Dialog.Root>
