<script lang="ts">
	import { onMount } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import * as Select from '$lib/components/ui/select';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { LoadStatus } from '$lib/load-status';
	import { ProfileOptions } from './profile-options.svelte';

	/** A quality profile picker. The caller saves the choice, e.g. Catalog's PATCH /titles/{id}. */
	let {
		value,
		onChange,
		id,
		disabled = false
	}: {
		value: string | undefined;
		onChange: (profileId: string) => void;
		id?: string;
		disabled?: boolean;
	} = $props();

	const options = new ProfileOptions();

	onMount(() => {
		options.load();
	});
</script>

{#if options.status === LoadStatus.LOADING}
	<Skeleton class="h-9 w-full" />
{:else if options.status === LoadStatus.ERROR}
	<div class="flex flex-wrap items-center gap-2">
		<span class="text-sm text-destructive">{options.error}</span>
		<Button size="sm" variant="outline" onclick={() => options.load()}>Retry</Button>
	</div>
{:else}
	<Select.Root type="single" {value} {disabled} onValueChange={onChange}>
		<Select.Trigger {id} class="w-full">{options.name(value) ?? 'Pick a profile'}</Select.Trigger>
		<Select.Content>
			{#each options.profiles as profile (profile.id)}
				<Select.Item value={profile.id} label={profile.name} />
			{/each}
		</Select.Content>
	</Select.Root>
{/if}
