<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { APP_LABELS, PATH_ACCESS_LABELS, PathAccess, type ArrApp } from './migration';
	import type { MigrationWizard } from './migration-wizard.svelte';

	let { wizard }: { wizard: MigrationWizard } = $props();

	const paths = $derived(wizard.state?.paths ?? []);

	async function save() {
		if (await wizard.savePaths()) wizard.next();
	}
</script>

<div class="grid gap-4">
	<p class="text-sm text-muted-foreground">
		Where Magpie sees each root folder. Change the start of the path when the apps run in containers
		with other mounts, e.g. <span class="font-mono">/tv</span> →
		<span class="font-mono">/data/media/tv</span>.
	</p>
	{#if paths.length === 0}
		<p class="text-sm text-muted-foreground">The connected apps list no root folders.</p>
	{:else}
		<ul class="grid gap-3">
			{#each paths as path (path.app + path.from)}
				{@const access = wizard.access(path.app, path.from)}
				<li
					class="grid gap-2 rounded-lg border p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)] sm:items-center sm:gap-4"
				>
					<div class="min-w-0">
						<p class="text-xs text-muted-foreground">{APP_LABELS[path.app as ArrApp]} sees</p>
						<p class="truncate font-mono text-sm">{path.from}</p>
					</div>
					<div class="grid min-w-0 gap-1.5">
						<label class="text-xs text-muted-foreground" for="path-{path.app}-{path.from}"
							>Magpie sees</label
						>
						<div class="flex min-w-0 items-center gap-2">
							<Input
								id="path-{path.app}-{path.from}"
								class="min-w-0 font-mono"
								value={wizard.pathDraft(path.app, path.from)}
								oninput={(event) => wizard.editPath(path.app, path.from, event.currentTarget.value)}
							/>
							{#if access}
								<Badge
									variant={access === PathAccess.OK ? 'secondary' : 'destructive'}
									class="shrink-0"
								>
									{PATH_ACCESS_LABELS[access as PathAccess] ?? access}
								</Badge>
							{/if}
						</div>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
	{#if !wizard.pathsReachable}
		<p class="text-xs text-muted-foreground">
			A folder Magpie can't reach is fine if nothing you import lives there; otherwise the commit
			stops at root folders.
		</p>
	{/if}
	<div class="flex justify-end">
		<Button disabled={wizard.busy} onclick={save}
			>{wizard.busy ? 'Saving…' : 'Save and continue'}</Button
		>
	</div>
</div>
