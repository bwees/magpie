<script lang="ts">
	import FileVideo from '@lucide/svelte/icons/file-video';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { LoadStatus } from '$lib/load-status';
	import type { MediaFileResponse } from '$lib/sdk';
	import FileCard from './FileCard.svelte';
	import type { TitleFiles } from './title-files.svelte';

	let {
		files,
		list = files.files
	}: {
		files: TitleFiles;
		/** The files to show; all of the title's by default. */
		list?: MediaFileResponse[];
	} = $props();

	const SKELETON_CARDS = 2;
</script>

{#if files.status === LoadStatus.LOADING}
	<div class="grid gap-2" aria-busy="true" aria-label="Loading files">
		{#each { length: SKELETON_CARDS }, i (i)}
			<Skeleton class="h-16 w-full rounded-xl" />
		{/each}
	</div>
{:else if files.status === LoadStatus.ERROR}
	<section
		class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-6 text-center"
		role="alert"
	>
		<p class="text-sm text-muted-foreground">{files.error}</p>
		<Button variant="outline" size="sm" onclick={() => files.load()}>Retry</Button>
	</section>
{:else if list.length === 0}
	<section class="flex items-center justify-center gap-2 rounded-xl border border-dashed p-6">
		<FileVideo class="size-5 text-muted-foreground" />
		<p class="text-sm text-muted-foreground">No files yet.</p>
	</section>
{:else}
	<div class="grid gap-2">
		{#each list as file (file.id)}
			<FileCard {file} {files} />
		{/each}
	</div>
{/if}

<AlertDialog.Root
	open={files.confirmingDelete !== undefined}
	onOpenChange={(open) => {
		if (!open) files.cancelDelete();
	}}
>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Delete this file?</AlertDialog.Title>
			<AlertDialog.Description>
				Magpie deletes the video from disk along with its subtitles. A monitored title is searched
				for again in the next missing sweep.
			</AlertDialog.Description>
		</AlertDialog.Header>
		<p class="font-mono text-xs break-all">{files.confirmingDelete?.path}</p>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
			<AlertDialog.Action
				class="bg-destructive text-white hover:bg-destructive/90"
				onclick={() => files.confirmDelete()}
			>
				Delete file
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
