import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	getAgentSecret,
	getTranscodeSettings,
	updateTranscodeSettings,
	type AgentSecretResponse,
	type TranscodeSettingsResponse,
	type UpdateTranscodeSettingsRequest
} from '$lib/sdk';
import { DEFAULT_SLOTS_PER_GPU, TRANSFER_CAP } from './agents';
import { inRange, toUpdate } from './transcode-settings';
import { dockerRunCommand } from './docker-run';
import { LoadState, type Notifier } from './agents-manager.svelte';

export interface AgentSettingsApi {
	secret: () => Promise<AgentSecretResponse>;
	settings: () => Promise<TranscodeSettingsResponse>;
	save: (settings: UpdateTranscodeSettingsRequest) => Promise<TranscodeSettingsResponse>;
}

export interface Clipboard {
	writeText: (text: string) => Promise<void>;
}

const sdkApi: AgentSettingsApi = {
	secret: () => getAgentSecret(),
	settings: () => getTranscodeSettings(),
	save: (settings) => updateTranscodeSettings(settings)
};

/** State and actions behind Settings → Agents. */
export class AgentSettingsManager {
	loadState = $state(LoadState.LOADING);
	error = $state('');
	secret = $state<AgentSecretResponse>({ configured: false, masked: '', secret: '' });
	/** The whole Transcode settings; this page edits only the agent limits and sends the rest back. */
	saved = $state<TranscodeSettingsResponse | null>(null);
	transferCap = $state<number>(TRANSFER_CAP.MIN);
	defaultSlotsPerGpu = $state<number>(DEFAULT_SLOTS_PER_GPU.MIN);
	saving = $state(false);

	readonly #api: AgentSettingsApi;
	readonly #notify: Notifier;
	readonly #clipboard: () => Clipboard;
	/** The sample shows the placeholder, never the secret itself. */
	readonly dockerRun: string;

	constructor(
		serverUrl: string,
		api: AgentSettingsApi = sdkApi,
		notify: Notifier = toast,
		clipboard: () => Clipboard = () => navigator.clipboard
	) {
		this.dockerRun = dockerRunCommand(serverUrl);
		this.#api = api;
		this.#notify = notify;
		this.#clipboard = clipboard;
	}

	readonly transferCapValid = $derived(inRange(this.transferCap, TRANSFER_CAP));
	readonly defaultSlotsValid = $derived(inRange(this.defaultSlotsPerGpu, DEFAULT_SLOTS_PER_GPU));
	readonly dirty = $derived(
		this.saved !== null &&
			(this.transferCap !== this.saved.transferCap ||
				this.defaultSlotsPerGpu !== this.saved.defaultSlotsPerGpu)
	);
	readonly canSave = $derived(
		this.dirty && this.transferCapValid && this.defaultSlotsValid && !this.saving
	);

	async load(): Promise<void> {
		try {
			const [secret, settings] = await Promise.all([this.#api.secret(), this.#api.settings()]);
			this.secret = secret;
			this.#apply(settings);
			this.loadState = LoadState.READY;
		} catch (error) {
			this.loadState = LoadState.ERROR;
			this.error = apiErrorMessage(error, 'Could not load agent settings.');
		}
	}

	async save(): Promise<void> {
		if (!this.canSave || !this.saved) return;
		this.saving = true;
		try {
			this.#apply(
				await this.#api.save({
					...toUpdate(this.saved),
					transferCap: this.transferCap,
					defaultSlotsPerGpu: this.defaultSlotsPerGpu
				})
			);
			this.#notify.success('Agent settings saved.');
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not save agent settings.'));
		} finally {
			this.saving = false;
		}
	}

	copySecret(): Promise<void> {
		return this.#copy(this.secret.secret, 'Secret copied.');
	}

	copyDockerRun(): Promise<void> {
		return this.#copy(this.dockerRun, 'Command copied.');
	}

	async #copy(text: string, done: string): Promise<void> {
		try {
			await this.#clipboard().writeText(text);
			this.#notify.success(done);
		} catch {
			this.#notify.error('Could not copy. Select the text and copy it by hand.');
		}
	}

	#apply(settings: TranscodeSettingsResponse): void {
		this.saved = settings;
		this.transferCap = settings.transferCap;
		this.defaultSlotsPerGpu = settings.defaultSlotsPerGpu;
	}
}
