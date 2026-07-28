import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ url }) => {
	if (url.searchParams.get('tab') === 'metadata') {
		throw redirect(302, '/library');
	}
};
