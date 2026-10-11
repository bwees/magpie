import adapter from '@sveltejs/adapter-static';
import type { Config } from '@sveltejs/kit';

const SPA_FALLBACK = 'index.html';

const config: Config = {
	compilerOptions: {
		runes: ({ filename }) => (filename.split(/[/\\]/).includes('node_modules') ? undefined : true)
	},
	kit: {
		adapter: adapter({
			fallback: SPA_FALLBACK
		})
	}
};

export default config;
