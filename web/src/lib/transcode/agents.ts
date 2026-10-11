import type { AgentResponse } from '$lib/sdk';

/** Live event types Transcode publishes about agents. */
export enum AgentEvent {
	ONLINE = 'transcode.AgentOnline',
	OFFLINE = 'transcode.AgentOffline'
}

export enum TransferMode {
	DOWNLOAD_FIRST = 'DOWNLOAD_FIRST',
	DIRECT_READ = 'DIRECT_READ'
}

export const TRANSFER_MODE_LABELS: Record<TransferMode, string> = {
	[TransferMode.DOWNLOAD_FIRST]: 'Download first',
	[TransferMode.DIRECT_READ]: 'Direct read'
};

/** Capabilities the server reports, as agentproto names them. */
export enum Capability {
	ENC_HEVC_10 = 'ENC_HEVC_10',
	ENC_AV1_10 = 'ENC_AV1_10',
	WHISPER_VULKAN = 'WHISPER_VULKAN'
}

/** The ✓/✗ columns on an agent card. */
export const CARD_CAPABILITIES: { capability: Capability; label: string }[] = [
	{ capability: Capability.ENC_HEVC_10, label: 'HEVC 10' },
	{ capability: Capability.ENC_AV1_10, label: 'AV1' },
	{ capability: Capability.WHISPER_VULKAN, label: 'Whisper' }
];

/** Bounds from 12-settings › Agents. A per-agent override of 0 drains through its slots. */
export const SLOTS_OVERRIDE = { MIN: 0, MAX: 8 } as const;
export const TRANSFER_CAP = { MIN: 1, MAX: 16 } as const;
export const DEFAULT_SLOTS_PER_GPU = { MIN: 1, MAX: 4 } as const;

export enum CapabilityStatus {
	PASSED = 'PASSED',
	FAILED = 'FAILED',
	UNTESTED = 'UNTESTED'
}

export interface CapabilitySummary {
	status: CapabilityStatus;
	/** Why every device failed, for the tooltip. */
	errors: string[];
}

/** An agent passes a capability when any of its devices passed it. */
export function summarizeCapability(
	agent: AgentResponse,
	capability: Capability
): CapabilitySummary {
	const results = agent.devices.flatMap((d) =>
		d.capabilities.filter((c) => c.capability === capability)
	);
	if (results.length === 0) return { status: CapabilityStatus.UNTESTED, errors: [] };
	if (results.some((r) => r.ok)) return { status: CapabilityStatus.PASSED, errors: [] };
	return { status: CapabilityStatus.FAILED, errors: results.map((r) => r.error).filter(Boolean) };
}

export function slotRange(): number[] {
	return Array.from(
		{ length: SLOTS_OVERRIDE.MAX - SLOTS_OVERRIDE.MIN + 1 },
		(_, i) => SLOTS_OVERRIDE.MIN + i
	);
}

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
const BIT_UNITS = ['bit/s', 'kbit/s', 'Mbit/s', 'Gbit/s'];
const UNIT_STEP = 1000;
const BITS_PER_BYTE = 8;
const ONE_DECIMAL_BELOW = 10;

function scaled(value: number, units: string[]): string {
	const power = value < UNIT_STEP ? 0 : Math.floor(Math.log(value) / Math.log(UNIT_STEP));
	const unit = Math.min(power, units.length - 1);
	const amount = value / UNIT_STEP ** unit;
	const digits = amount < ONE_DECIMAL_BELOW && unit > 0 ? 1 : 0;
	return `${amount.toFixed(digits)} ${units[unit]}`;
}

export function formatBytes(bytes: number): string {
	return scaled(bytes, BYTE_UNITS);
}

/** Link speed from the agent's EWMA throughput in bytes per second; 0 means not measured yet. */
export function formatLinkSpeed(bytesPerSecond: number): string {
	return bytesPerSecond > 0 ? scaled(bytesPerSecond * BITS_PER_BYTE, BIT_UNITS) : '—';
}
