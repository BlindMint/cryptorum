import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	build: {
		rollupOptions: {
			output: {
				manualChunks(id) {
					if (!id.includes('node_modules')) return;
					if (id.includes('chart.js')) return 'vendor-charts';
					if (id.includes('epubjs') || id.includes('@xmldom') || id.includes('lodash')) {
						return 'vendor-epub';
					}
				}
			}
		}
	},
	server: {
		proxy: {
			'/api': 'http://localhost:6060'
		}
	}
});
