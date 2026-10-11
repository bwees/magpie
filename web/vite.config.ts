import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';

const DEFAULT_API_TARGET = 'http://127.0.0.1:3000';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		proxy: {
			'/api': {
				target: process.env.API_PROXY_TARGET ?? DEFAULT_API_TARGET,
				ws: true,
				changeOrigin: true
			}
		}
	},
	test: {
		include: ['src/**/*.test.ts']
	}
});
