<script lang="ts">
	import Download from '@lucide/svelte/icons/download';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { LoadStatus } from '$lib/load-status';
	import { cn } from '$lib/utils';
	import type { FileSubtitles } from './file-subtitles.svelte';
	import { languageName } from './subtitles';

	let { subs }: { subs: FileSubtitles } = $props();

	const SKELETON_ROWS = 3;
</script>

<Dialog.Root
	open={subs.pickerOpen}
	onOpenChange={(open) => {
		if (!open) subs.closePicker();
	}}
>
	<Dialog.Content class="sm:max-w-3xl">
		<Dialog.Header>
			<Dialog.Title>Find subtitles</Dialog.Title>
			<Dialog.Description>
				OpenSubtitles results, best match first. Downloading uses one of today’s downloads and syncs
				the subtitle when auto-sync is on.
			</Dialog.Description>
		</Dialog.Header>

		<div class="flex flex-wrap items-center gap-4">
			<Select.Root
				type="single"
				value={subs.pickerLanguage}
				onValueChange={(language) => language && subs.choose(language, subs.pickerForced)}
			>
				<Select.Trigger class="w-full sm:w-48" aria-label="Language">
					{languageName(subs.pickerLanguage)}
				</Select.Trigger>
				<Select.Content>
					{#each subs.languages as language (language)}
						<Select.Item value={language} label={languageName(language)} />
					{/each}
				</Select.Content>
			</Select.Root>
			<div class="flex items-center gap-1.5">
				<Checkbox
					id="picker-forced"
					checked={subs.pickerForced}
					onCheckedChange={(on) => subs.choose(subs.pickerLanguage, on === true)}
				/>
				<Label for="picker-forced" class="text-sm font-normal">Forced (foreign parts only)</Label>
			</div>
		</div>

		<div class="max-h-[60vh] overflow-y-auto">
			{#if subs.candidatesStatus === LoadStatus.LOADING}
				<div class="grid gap-2" aria-busy="true" aria-label="Searching OpenSubtitles">
					{#each { length: SKELETON_ROWS }, i (i)}
						<Skeleton class="h-14 w-full" />
					{/each}
				</div>
			{:else if subs.candidatesStatus === LoadStatus.ERROR}
				<div class="flex flex-wrap items-center gap-3" role="alert">
					<p class="text-sm text-destructive">{subs.candidatesError}</p>
					<Button size="sm" variant="outline" onclick={() => subs.loadCandidates()}>Retry</Button>
				</div>
			{:else if subs.candidates.length === 0}
				<p class="py-6 text-center text-sm text-muted-foreground">
					OpenSubtitles has nothing for {languageName(subs.pickerLanguage)}.
				</p>
			{:else}
				<ul class="grid gap-2" aria-label="Candidates">
					{#each subs.candidates as c (c.fileId)}
						<li
							class={cn(
								'flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:gap-3',
								!c.acceptable && 'opacity-75'
							)}
						>
							<div class="grid min-w-0 flex-1 gap-1">
								<span class="font-mono text-xs break-all">{c.releaseName || 'No release name'}</span
								>
								<span class="flex flex-wrap items-center gap-1.5">
									<span
										class={cn(
											'text-xs font-medium tabular-nums',
											c.acceptable ? 'text-primary' : 'text-muted-foreground'
										)}
										title="{c.points} of {c.maxPoints} points">{c.score}%</span
									>
									{#if !c.acceptable}
										<span class="text-xs text-muted-foreground">below the minimum</span>
									{/if}
									{#if c.hashMatch}<Badge variant="outline" class="border-primary/50 text-primary"
											>Hash match</Badge
										>{/if}
									{#if c.hearingImpaired}<Badge variant="secondary">HI</Badge>{/if}
									{#if c.forced}<Badge variant="secondary">Forced</Badge>{/if}
									{#if c.aiTranslated}<Badge variant="secondary">AI translated</Badge>{/if}
								</span>
							</div>
							<Button
								size="sm"
								class="self-end sm:self-auto"
								disabled={subs.downloading !== undefined}
								onclick={() => subs.download(c)}
							>
								<Download />{subs.downloading === c.fileId ? 'Downloading…' : 'Download'}
							</Button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</Dialog.Content>
</Dialog.Root>
