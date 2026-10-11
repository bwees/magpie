<script lang="ts">
	import type { Snippet } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Input } from '$lib/components/ui/input';
	import * as Select from '$lib/components/ui/select';
	import { Separator } from '$lib/components/ui/separator';
	import { filterSections, sectionFor, settingsPath } from '$lib/settings/sections';
	import { cn } from '$lib/utils';

	let { children }: { children: Snippet } = $props();

	let query = $state('');
	const sections = $derived(filterSections(query));
	const current = $derived(sectionFor(page.url.pathname));
</script>

<svelte:head>
	<title>{current?.label ?? 'Settings'} · Settings · Magpie</title>
</svelte:head>

<div class="flex flex-col gap-4 md:flex-row md:gap-8">
	<nav aria-label="Settings sections" class="md:w-52 md:shrink-0">
		<Input
			type="search"
			bind:value={query}
			placeholder="Search settings…"
			aria-label="Search settings"
			class="mb-3 h-8"
		/>
		<div class="md:hidden">
			<Select.Root
				type="single"
				value={current?.section}
				onValueChange={(section) => {
					const spec = sections.find((s) => s.section === section);
					if (spec) goto(settingsPath(spec.section));
				}}
			>
				<Select.Trigger class="w-full" aria-label="Settings section">
					{current?.label ?? 'Choose a section'}
				</Select.Trigger>
				<Select.Content>
					{#each sections as spec (spec.section)}
						<Select.Item value={spec.section} label={spec.label} />
					{/each}
				</Select.Content>
			</Select.Root>
		</div>
		<ul class="hidden md:grid md:gap-0.5">
			{#each sections as spec (spec.section)}
				<li>
					<a
						href={settingsPath(spec.section)}
						aria-current={current?.section === spec.section ? 'page' : undefined}
						class={cn(
							'block rounded-md px-3 py-1.5 text-sm hover:bg-muted',
							current?.section === spec.section && 'bg-muted font-medium'
						)}
					>
						{spec.label}
					</a>
				</li>
			{:else}
				<li class="px-3 py-1.5 text-sm text-muted-foreground">No matching settings.</li>
			{/each}
		</ul>
	</nav>

	<div class="max-w-3xl min-w-0 flex-1">
		{#if current}
			<h2 class="hidden text-xl font-semibold md:block">{current.label}</h2>
			<Separator class="my-4 hidden md:block" />
		{/if}
		{@render children()}
	</div>
</div>
