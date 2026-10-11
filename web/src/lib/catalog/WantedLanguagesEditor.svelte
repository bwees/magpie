<script lang="ts">
	import ArrowDown from '@lucide/svelte/icons/arrow-down';
	import ArrowUp from '@lucide/svelte/icons/arrow-up';
	import GripVertical from '@lucide/svelte/icons/grip-vertical';
	import X from '@lucide/svelte/icons/x';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Select from '$lib/components/ui/select';
	import { cn } from '$lib/utils';
	import { LanguageFlag, type WantedLanguageList } from './wanted-languages.svelte';

	let { list, disabled = false }: { list: WantedLanguageList; disabled?: boolean } = $props();

	let adding = $state('');

	function add(code: string) {
		list.add(code);
		adding = '';
	}
</script>

<div class="grid gap-3">
	<ol class="grid gap-2" aria-label="Wanted languages, most wanted first">
		{#each list.languages as entry, position (entry.language)}
			<li
				draggable={!disabled}
				ondragstart={() => list.startDrag(position)}
				ondragover={(event) => event.preventDefault()}
				ondrop={(event) => {
					event.preventDefault();
					list.dropOn(position);
				}}
				ondragend={() => (list.dragging = undefined)}
				class={cn(
					'flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border bg-card px-3 py-2',
					list.dragging === position && 'opacity-50'
				)}
			>
				<GripVertical
					class={cn('size-4 shrink-0 text-muted-foreground', !disabled && 'cursor-grab')}
					aria-hidden="true"
				/>
				<span class="w-4 text-xs text-muted-foreground tabular-nums">{position + 1}</span>
				<span class="min-w-32 flex-1 truncate text-sm font-medium">
					{list.nameOf(entry.language)}
					<span class="ml-1 font-mono text-xs text-muted-foreground">{entry.language}</span>
				</span>
				<div class="flex items-center gap-3">
					<label class="flex items-center gap-1.5 text-xs">
						<Checkbox
							checked={entry.forced}
							{disabled}
							onCheckedChange={() => list.toggle(position, LanguageFlag.FORCED)}
						/>
						Forced
					</label>
					<label
						class="flex items-center gap-1.5 text-xs"
						title="Hearing-impaired subtitles are fine"
					>
						<Checkbox
							checked={entry.hearingImpaired}
							{disabled}
							onCheckedChange={() => list.toggle(position, LanguageFlag.HEARING_IMPAIRED)}
						/>
						HI
					</label>
					<div class="flex">
						<Button
							size="icon-sm"
							variant="ghost"
							aria-label="Move {list.nameOf(entry.language)} up"
							disabled={disabled || position === 0}
							onclick={() => list.move(position, position - 1)}
						>
							<ArrowUp />
						</Button>
						<Button
							size="icon-sm"
							variant="ghost"
							aria-label="Move {list.nameOf(entry.language)} down"
							disabled={disabled || position === list.languages.length - 1}
							onclick={() => list.move(position, position + 1)}
						>
							<ArrowDown />
						</Button>
						<Button
							size="icon-sm"
							variant="ghost"
							aria-label="Remove {list.nameOf(entry.language)}"
							disabled={disabled || !list.canRemove}
							onclick={() => list.remove(position)}
						>
							<X />
						</Button>
					</div>
				</div>
			</li>
		{/each}
	</ol>

	<Select.Root type="single" bind:value={adding} onValueChange={(code) => code && add(code)}>
		<Select.Trigger class="w-full sm:w-64" {disabled} aria-label="Add a language">
			Add language…
		</Select.Trigger>
		<Select.Content class="max-h-72">
			{#each list.addable as option (option.code)}
				<Select.Item value={option.code} label="{option.name} ({option.code})" />
			{/each}
		</Select.Content>
	</Select.Root>
</div>
