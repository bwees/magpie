export const AGENT_IMAGE = 'ghcr.io/bwees/magpie-agent:latest';
export const SECRET_PLACEHOLDER = '<MAGPIE_AGENT_SECRET>';

const LINE_BREAK = ' \\\n  ';

/**
 * A sample command that starts an agent pointed at this server. It passes every GPU vendor through:
 * NVIDIA needs --gpus with the video driver capability, Intel and AMD need the render nodes.
 */
export function dockerRunCommand(serverUrl: string, secret = SECRET_PLACEHOLDER): string {
	return [
		'docker run -d --name magpie-agent --restart unless-stopped',
		'--gpus all -e NVIDIA_DRIVER_CAPABILITIES=compute,video,utility',
		'--device /dev/dri --group-add "$(getent group render | cut -d: -f3)"',
		`-e MAGPIE_URL=${serverUrl}`,
		`-e MAGPIE_AGENT_SECRET=${secret}`,
		'-v magpie-agent-scratch:/scratch',
		AGENT_IMAGE
	].join(LINE_BREAK);
}
