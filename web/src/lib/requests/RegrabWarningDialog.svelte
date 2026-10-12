<script lang="ts">
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Button } from '$lib/components/ui/button';
	import type { IssueFix } from './issue-fix.svelte';

	let { fix }: { fix: IssueFix } = $props();
</script>

<AlertDialog.Root
	open={fix.held !== undefined}
	onOpenChange={(open) => {
		if (!open) fix.dismiss();
	}}
>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title class="flex items-center gap-2">
				<TriangleAlert class="size-5 shrink-0 text-amber-500" />
				A download is waiting for you
			</AlertDialog.Title>
			<AlertDialog.Description>
				{fix.held?.download.message}. Re-grabbing now would find it still queued. Map its files
				instead, or remove it and continue.
			</AlertDialog.Description>
		</AlertDialog.Header>
		{#if fix.held}
			<p class="text-sm text-muted-foreground">
				Remove it and continue deletes that download with its files, blocklists its release and the
				release of the file reported for <span class="text-foreground">{fix.held.subject}</span>,
				then searches again.
			</p>
		{/if}
		<AlertDialog.Footer class="gap-2">
			<AlertDialog.Cancel disabled={fix.continuing}>Cancel</AlertDialog.Cancel>
			<Button variant="outline" href={fix.releasePath} onclick={() => fix.dismiss()}>
				View the release
			</Button>
			<Button
				variant="destructive"
				disabled={fix.continuing}
				onclick={() => fix.removeAndContinue()}
			>
				Remove it and continue
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
