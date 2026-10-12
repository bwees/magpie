<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { APP_LABELS, APP_PLACEHOLDERS, ARR_APPS } from './migration';
	import type { MigrationWizard } from './migration-wizard.svelte';

	let { wizard }: { wizard: MigrationWizard } = $props();
</script>

<div class="grid gap-4">
	<p class="text-sm text-muted-foreground">
		Magpie reads Sonarr 4 and Radarr through their API, read-only. Connect either or both; skip one
		you don't use.
	</p>
	{#each ARR_APPS as app (app)}
		{@const form = wizard.forms[app]}
		{@const source = wizard.source(app)}
		<section class="grid gap-3 rounded-lg border p-3 sm:p-4" aria-label={APP_LABELS[app]}>
			<div class="flex flex-wrap items-center gap-2">
				<h3 class="font-medium">{APP_LABELS[app]}</h3>
				{#if source?.skipped}
					<Badge variant="outline">Skipped</Badge>
				{:else if source}
					<Badge variant="secondary">Connected · v{source.version}</Badge>
				{/if}
			</div>
			<div class="grid gap-3 sm:grid-cols-2">
				<div class="grid min-w-0 gap-1.5">
					<Label for="{app}-url">URL</Label>
					<Input
						id="{app}-url"
						bind:value={form.url}
						placeholder={APP_PLACEHOLDERS[app]}
						disabled={form.busy}
					/>
				</div>
				<div class="grid min-w-0 gap-1.5">
					<Label for="{app}-key">API key</Label>
					<Input
						id="{app}-key"
						type="password"
						autocomplete="off"
						bind:value={form.apiKey}
						placeholder={source && !source.skipped ? 'Saved' : 'Settings → General → Security'}
						disabled={form.busy}
					/>
				</div>
			</div>
			{#if form.error}
				<p class="text-sm text-destructive" role="alert">{form.error}</p>
			{/if}
			<div class="flex flex-wrap justify-end gap-2">
				<Button variant="ghost" disabled={form.busy} onclick={() => wizard.skipApp(app)}
					>Skip</Button
				>
				<Button
					disabled={form.busy || form.url.trim() === '' || form.apiKey.trim() === ''}
					onclick={() => wizard.connect(app)}
				>
					{form.busy ? 'Connecting…' : source && !source.skipped ? 'Reconnect' : 'Connect'}
				</Button>
			</div>
		</section>
	{/each}
	<div class="flex justify-end">
		<Button disabled={!wizard.connected} onclick={() => wizard.next()}>Continue</Button>
	</div>
</div>
