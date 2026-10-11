<script lang="ts">
	import type { TitleResponse } from '$lib/sdk';
	import PipelineBar from './PipelineBar.svelte';
	import PosterImage from './PosterImage.svelte';
	import { Button } from '$lib/components/ui/button';

	let { title, posterUrl, tint }: { title: TitleResponse; posterUrl?: string; tint?: string } =
		$props();

	/** A dark teal used until the poster colour is known. */
	const DEFAULT_TINT = '24 72 80';

	const background = $derived(
		`linear-gradient(120deg, rgb(${tint ?? DEFAULT_TINT} / 0.75), rgb(${tint ?? DEFAULT_TINT} / 0.25) 55%, var(--card))`
	);
	const chip = 'rounded-md bg-white/15 px-2 py-0.5';
	const placeholderAction = 'Searching arrives with Acquisition';
</script>

<section class="overflow-hidden rounded-xl border bg-card" style:background-image={background}>
	<div class="flex flex-col gap-4 p-4 text-white md:flex-row md:items-end md:gap-6 md:p-8 md:pt-12">
		<div class="flex items-end gap-4 md:contents">
			<PosterImage
				src={posterUrl}
				alt="{title.name} poster"
				class="w-24 shrink-0 ring-2 ring-white/20 md:w-36"
			/>
			<h1 class="text-2xl font-semibold tracking-tight md:hidden">{title.name}</h1>
		</div>
		<div class="min-w-0 flex-1">
			<h1 class="hidden text-4xl font-semibold tracking-tight md:block">{title.name}</h1>
			{#if title.overview}
				<p class="mt-2 line-clamp-3 max-w-3xl text-sm text-white/80 md:line-clamp-none">
					{title.overview}
				</p>
			{/if}
			<div class="mt-3 flex flex-wrap gap-2 text-xs">
				{#if title.year}
					<span class={chip}>{title.year}</span>
				{/if}
				{#if title.requestedBy}
					<span class={chip}>Requested</span>
				{/if}
				{#if !title.monitored}
					<span class={chip}>Not monitored</span>
				{/if}
			</div>
		</div>
		<div class="grid grid-cols-2 gap-2 md:flex md:shrink-0">
			<Button variant="secondary" disabled title={placeholderAction}>Search</Button>
			<Button
				variant="outline"
				class="border-white/30 bg-transparent text-white hover:bg-white/10"
				disabled
				title={placeholderAction}
			>
				Interactive search
			</Button>
		</div>
	</div>
	<div class="px-4 pb-5 md:px-8 md:pb-6">
		<PipelineBar />
	</div>
</section>
