<script lang="ts">
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import type { TitlePage } from './title-page.svelte';

	let { page, onRemoved }: { page: TitlePage; onRemoved: () => void } = $props();

	async function confirmRemove() {
		if (await page.remove()) {
			onRemoved();
		}
	}
</script>

{#if page.title}
	<div class="grid max-w-lg gap-6">
		<div class="flex items-center justify-between gap-4">
			<div>
				<label for="title-monitored" class="text-sm font-medium">Monitored</label>
				<p class="text-xs text-muted-foreground">
					Search automatically for missing files and upgrades
				</p>
			</div>
			<Switch
				id="title-monitored"
				checked={page.title.monitored}
				onCheckedChange={(checked) => page.setMonitored(checked)}
			/>
		</div>
		<div>
			<Button variant="destructive" onclick={() => page.askToRemove()}>Delete title…</Button>
		</div>
	</div>

	<AlertDialog.Root bind:open={page.confirmingRemove}>
		<AlertDialog.Content>
			<AlertDialog.Header>
				<AlertDialog.Title>Delete {page.title.name}?</AlertDialog.Title>
				<AlertDialog.Description>
					Magpie stops tracking this title. Files stay on disk unless you choose to delete them.
				</AlertDialog.Description>
			</AlertDialog.Header>
			<div class="flex items-center justify-between gap-4">
				<label for="delete-files" class="text-sm">Also delete its files from disk</label>
				<Switch id="delete-files" bind:checked={page.deleteFiles} />
			</div>
			<AlertDialog.Footer>
				<AlertDialog.Cancel disabled={page.removing}>Cancel</AlertDialog.Cancel>
				<AlertDialog.Action
					class="bg-destructive text-white hover:bg-destructive/90"
					disabled={page.removing}
					onclick={confirmRemove}
				>
					Delete
				</AlertDialog.Action>
			</AlertDialog.Footer>
		</AlertDialog.Content>
	</AlertDialog.Root>
{/if}
