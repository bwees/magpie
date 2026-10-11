import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';

const DEFAULT_API_TARGET = 'http://127.0.0.1:3000';
const apiTarget = process.env.API_PROXY_TARGET ?? DEFAULT_API_TARGET;

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		proxy: {
			'/api': {
				target: apiTarget,
				ws: true,
				changeOrigin: true,
				// The live socket only accepts its own origin, so present the proxied one as the server's.
				configure: (proxy) => {
					proxy.on('proxyReqWs', (proxyReq) => proxyReq.setHeader('origin', apiTarget));
				}
			}
		}
	},
	test: {
		include: ['src/**/*.test.ts']
	}
});
