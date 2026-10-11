import { describe, expect, it } from 'vitest';
import type { AgentDeviceResponse, AgentResponse } from '$lib/sdk';
import {
	Capability,
	CapabilityStatus,
	formatBytes,
	formatLinkSpeed,
	slotRange,
	SLOTS_OVERRIDE,
	summarizeCapability
} from './agents';
import { AGENT_IMAGE, dockerRunCommand, SECRET_PLACEHOLDER } from './docker-run';

const SERVER = 'http://magpie.lan:3000';
const SESSION_ERROR = 'OpenEncodeSessionEx failed';

function device(
	id: string,
	results: Partial<Record<Capability, string | true>>
): AgentDeviceResponse {
	return {
		id,
		vendor: 'NVIDIA',
		name: 'GPU',
		driver: '560',
		maxSessions: 0,
		healthy: true,
		slots: 1,
		capabilities: Object.entries(results).map(([capability, result]) => ({
			capability,
			ok: result === true,
			error: result === true ? '' : result,
			testedAt: ''
		}))
	};
}

function agent(...devices: AgentDeviceResponse[]): AgentResponse {
	return {
		id: 'A',
		hostname: 'gpu-box',
		version: '1',
		ffmpegVersion: '7',
		mode: 'DOWNLOAD_FIRST',
		slotsOverride: null,
		online: true,
		draining: false,
		lastSeenAt: '',
		scratchFreeBytes: 0,
		throughputBps: 0,
		lifetimeSavedBytes: 0,
		devices
	};
}

describe('summarizeCapability', () => {
	it('passes when any device passed', () => {
		const a = agent(
			device('cuda:0', { [Capability.ENC_AV1_10]: SESSION_ERROR }),
			device('cuda:1', { [Capability.ENC_AV1_10]: true })
		);
		expect(summarizeCapability(a, Capability.ENC_AV1_10)).toEqual({
			status: CapabilityStatus.PASSED,
			errors: []
		});
	});

	it('fails with every device error when none passed', () => {
		const a = agent(device('cuda:0', { [Capability.ENC_AV1_10]: SESSION_ERROR }));
		expect(summarizeCapability(a, Capability.ENC_AV1_10)).toEqual({
			status: CapabilityStatus.FAILED,
			errors: [SESSION_ERROR]
		});
	});

	it('is untested without results', () => {
		expect(summarizeCapability(agent(), Capability.WHISPER_VULKAN).status).toBe(
			CapabilityStatus.UNTESTED
		);
	});
});

describe('formatting', () => {
	it.each([
		[0, '0 B'],
		[999, '999 B'],
		[1_500_000, '1.5 MB'],
		[512_000_000_000, '512 GB']
	])('formats %d bytes as %s', (bytes, text) => {
		expect(formatBytes(bytes)).toBe(text);
	});

	it('shows link speed in bits and a dash before any transfer', () => {
		expect(formatLinkSpeed(0)).toBe('—');
		expect(formatLinkSpeed(125_000_000)).toBe('1.0 Gbit/s');
	});

	it('offers every slot override from drain to the maximum', () => {
		expect(slotRange()).toEqual([0, 1, 2, 3, 4, 5, 6, 7, 8]);
		expect(slotRange().at(-1)).toBe(SLOTS_OVERRIDE.MAX);
	});
});

describe('dockerRunCommand', () => {
	it('points the agent at the server with a secret placeholder', () => {
		const command = dockerRunCommand(SERVER);
		expect(command).toContain(`MAGPIE_URL=${SERVER}`);
		expect(command).toContain(`MAGPIE_AGENT_SECRET=${SECRET_PLACEHOLDER}`);
		expect(command).toContain('NVIDIA_DRIVER_CAPABILITIES=compute,video,utility');
		expect(command.endsWith(AGENT_IMAGE)).toBe(true);
	});
});
