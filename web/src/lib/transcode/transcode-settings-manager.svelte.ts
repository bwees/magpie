import { toast } from 'svelte-sonner';
import {
	apiErrorMessage,
	getTranscodeBacklog,
	getTranscodeSettings,
	sweepTranscodeBacklog,
	updateTranscodeSettings,
	type BacklogResponse,
	type EncoderCalibrationResponse,
	type TranscodeSettingsResponse,
	type UpdateTranscodeSettingsRequest
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import type { Notifier } from './gpu-jobs-manager.svelte';
import {
	BITRATE_CAP_MBPS,
	LOW_BITRATE_PCT,
	MIN_SAVING_PCT,
	Preset,
	ResolutionClass,
	TargetCodec,
	inRange,
	sameSettings,
	toUpdate
} from './transcode-settings';

export interface TranscodeSettingsApi {
	settings: () => Promise<TranscodeSettingsResponse>;
	save: (settings: UpdateTranscodeSettingsRequest) => Promise<TranscodeSettingsResponse>;
	backlog: () => Promise<BacklogResponse>;
	sweep: () => Promise<BacklogResponse>;
}

const sdkApi: TranscodeSettingsApi = {
	settings: () => getTranscodeSettings(),
	save: (settings) => updateTranscodeSettings(settings),
	backlog: () => getTranscodeBacklog(),
	sweep: () => sweepTranscodeBacklog()
};

/** An empty override box uses the calibrated value. */
type Override = number | null;

const EMPTY_BACKLOG: BacklogResponse = { backlog: 0, eligible: 0, skipped: {} };

/** State and actions behind Settings → Transcode. */
export class TranscodeSettingsManager {
	status = $state(LoadStatus.LOADING);
	error = $state('');
	saving = $state(false);
	sweeping = $state(false);
	advancedOpen = $state(false);
	saved = $state<TranscodeSettingsResponse | null>(null);
	backlog = $state<BacklogResponse>(EMPTY_BACKLOG);

	enabled = $state(true);
	targetCodec = $state<string>(TargetCodec.HEVC_10);
	preset = $state<string>(Preset.HIGH);
	caps = $state<Record<string, number>>({});
	skipAlreadyTarget = $state(true);
	skipLowBitrate = $state(true);
	lowBitratePct = $state<number>(LOW_BITRATE_PCT.MAX);
	minSavingPct = $state<number>(MIN_SAVING_PCT.MIN);
	overrides = $state<Record<string, Override>>({});

	readonly #api: TranscodeSettingsApi;
	readonly #notify: Notifier;

	constructor(api: TranscodeSettingsApi = sdkApi, notify: Notifier = toast) {
		this.#api = api;
		this.#notify = notify;
	}

	get encoders(): EncoderCalibrationResponse[] {
		return this.saved?.encoders ?? [];
	}

	capValid(resolution: ResolutionClass): boolean {
		return inRange(this.caps[resolution], BITRATE_CAP_MBPS);
	}

	overrideValid(encoder: EncoderCalibrationResponse): boolean {
		const value = this.overrides[encoder.encoder];
		return (
			value === null ||
			value === undefined ||
			inRange(value, { MIN: encoder.min, MAX: encoder.max })
		);
	}

	/** The calibrated value the encoder uses for the selected preset. */
	calibrated(encoder: EncoderCalibrationResponse): number | undefined {
		return encoder.presets[this.preset];
	}

	readonly lowBitrateValid = $derived(inRange(this.lowBitratePct, LOW_BITRATE_PCT));
	readonly minSavingValid = $derived(inRange(this.minSavingPct, MIN_SAVING_PCT));

	readonly valid = $derived(
		Object.values(ResolutionClass).every((r) => this.capValid(r)) &&
			this.lowBitrateValid &&
			this.minSavingValid &&
			this.encoders.every((e) => this.overrideValid(e))
	);

	readonly dirty = $derived(
		this.saved !== null && !sameSettings(this.#request(), toUpdate(this.saved))
	);

	readonly canSave = $derived(this.dirty && this.valid && !this.saving);

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		try {
			const [settings, backlog] = await Promise.all([this.#api.settings(), this.#api.backlog()]);
			this.#apply(settings);
			this.backlog = backlog;
			this.status = LoadStatus.READY;
		} catch (error) {
			this.error = apiErrorMessage(error, 'Could not load the transcode settings.');
			this.status = LoadStatus.ERROR;
		}
	}

	async save(): Promise<void> {
		if (!this.canSave) return;
		this.saving = true;
		try {
			this.#apply(await this.#api.save(this.#request()));
			this.#notify.success('Transcode settings saved.');
			this.backlog = await this.#api.backlog();
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not save the transcode settings.'));
		} finally {
			this.saving = false;
		}
	}

	discard(): void {
		if (this.saved) this.#apply(this.saved);
	}

	async rescan(): Promise<void> {
		this.sweeping = true;
		try {
			this.backlog = await this.#api.sweep();
			this.#notify.success('Re-scanning the library against the transcode policy.');
		} catch (error) {
			this.#notify.error(apiErrorMessage(error, 'Could not start the re-scan.'));
		} finally {
			this.sweeping = false;
		}
	}

	#request(): UpdateTranscodeSettingsRequest {
		const encoderOverrides: Record<string, number> = {};
		for (const [encoder, value] of Object.entries(this.overrides)) {
			if (value !== null && value !== undefined) encoderOverrides[encoder] = value;
		}
		return {
			enabled: this.enabled,
			targetCodec: this.targetCodec,
			preset: this.preset,
			bitrateCapsMbps: { ...this.caps },
			skipAlreadyTarget: this.skipAlreadyTarget,
			skipLowBitrate: this.skipLowBitrate,
			lowBitratePct: this.lowBitratePct,
			minSavingPct: this.minSavingPct,
			encoderOverrides,
			transferCap: this.saved?.transferCap ?? 0,
			defaultSlotsPerGpu: this.saved?.defaultSlotsPerGpu ?? 0
		};
	}

	#apply(settings: TranscodeSettingsResponse): void {
		this.saved = settings;
		this.enabled = settings.enabled;
		this.targetCodec = settings.targetCodec;
		this.preset = settings.preset;
		this.caps = { ...settings.bitrateCapsMbps };
		this.skipAlreadyTarget = settings.skipAlreadyTarget;
		this.skipLowBitrate = settings.skipLowBitrate;
		this.lowBitratePct = settings.lowBitratePct;
		this.minSavingPct = settings.minSavingPct;
		this.overrides = Object.fromEntries(
			settings.encoders.map((e) => [e.encoder, settings.encoderOverrides[e.encoder] ?? null])
		);
	}
}
