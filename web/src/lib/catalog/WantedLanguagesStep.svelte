<script lang="ts">
	import { onMount } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { LoadStatus } from '$lib/load-status';
	import WantedLanguagesEditor from './WantedLanguagesEditor.svelte';
	import { GlobalLanguages } from './wanted-languages.svelte';

	let { isLast, onDone, onSkip }: { isLast: boolean; onDone: () => void; onSkip: () => void } =
		$props();

	const languages = new GlobalLanguages();

	onMount(() => {
		languages.load();
	});

	/** Saves even an unchanged list, so finishing the step always confirms it. */
	async function save() {
		if (await languages.save()) onDone();
	}
</script>

<Card.Content>
	{#if languages.status === LoadStatus.LOADING}
		<Skeleton class="h-24 w-full" />
	{:else if languages.status === LoadStatus.ERROR}
		<div class="flex flex-wrap items-center gap-3">
			<p class="text-sm text-destructive">{languages.error}</p>
			<Button size="sm" variant="outline" onclick={() => languages.load()}>Retry</Button>
		</div>
	{:else}
		<WantedLanguagesEditor list={languages.list} disabled={languages.saving} />
	{/if}
</Card.Content>
<Card.Footer class="flex-wrap justify-end gap-2">
	<Button variant="ghost" disabled={languages.saving} onclick={onSkip}>Skip</Button>
	<Button disabled={languages.saving || languages.status !== LoadStatus.READY} onclick={save}>
		{languages.saving ? 'Saving…' : isLast ? 'Finish' : 'Continue'}
	</Button>
</Card.Footer>
