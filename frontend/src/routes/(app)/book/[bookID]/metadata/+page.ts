import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ params, url }) => {
	const searchParams = new URLSearchParams(url.searchParams);
	searchParams.set('edit', 'metadata');
	throw redirect(302, `/book/${params.bookID}?${searchParams.toString()}`);
};
