<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import FileList from './FileList.svelte';
	import type { TitleFiles } from './title-files.svelte';

	let { files }: { files: TitleFiles } = $props();
</script>

<Dialog.Root
	open={files.openEpisode !== undefined}
	onOpenChange={(open) => {
		if (!open) files.closeEpisode();
	}}
>
	<Dialog.Content class="flex max-h-[85vh] flex-col gap-4 sm:max-w-3xl">
		{#if files.openEpisode}
			<Dialog.Header>
				<Dialog.Title>{files.openEpisode.label}</Dialog.Title>
			</Dialog.Header>
			<div class="-mx-6 min-h-0 overflow-y-auto px-6 pb-1">
				<FileList {files} list={files.forEpisode(files.openEpisode.id)} />
			</div>
		{/if}
	</Dialog.Content>
</Dialog.Root>
