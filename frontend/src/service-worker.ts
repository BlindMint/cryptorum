/// <reference lib="webworker" />

import { build, files, version } from '$service-worker';

const worker = self as unknown as ServiceWorkerGlobalScope;

const APP_CACHE = `cryptorum-app-${version}`;
const STATIC_CACHE = `cryptorum-static-${version}`;
const CACHES = [APP_CACHE, STATIC_CACHE];
const APP_SHELL = '/';
const BYPASS_PATHS = ['/api/', '/opds/', '/kobo/'];

const appAssets = [APP_SHELL, ...build];
const staticAssets = files.filter((asset) => !asset.endsWith('.map'));

function sameOrigin(url: URL): boolean {
	return url.origin === worker.location.origin;
}

function shouldBypass(url: URL): boolean {
	return !sameOrigin(url) || BYPASS_PATHS.some((path) => url.pathname.startsWith(path));
}

async function cacheAll(cacheName: string, assets: string[]): Promise<void> {
	const cache = await caches.open(cacheName);
	await cache.addAll(assets);
}

worker.addEventListener('install', (event) => {
	event.waitUntil(
		Promise.all([cacheAll(APP_CACHE, appAssets), cacheAll(STATIC_CACHE, staticAssets)]).then(() =>
			worker.skipWaiting()
		)
	);
});

worker.addEventListener('activate', (event) => {
	event.waitUntil(
		caches
			.keys()
			.then((keys) =>
				Promise.all(keys.filter((key) => !CACHES.includes(key)).map((key) => caches.delete(key)))
			)
			.then(() => worker.clients.claim())
	);
});

worker.addEventListener('fetch', (event) => {
	const { request } = event;

	if (request.method !== 'GET') {
		return;
	}

	const url = new URL(request.url);
	if (shouldBypass(url)) {
		return;
	}

	if (request.mode === 'navigate') {
		event.respondWith(
			fetch(request)
				.then((response) => {
					if (response.ok) {
						const copy = response.clone();
						caches.open(APP_CACHE).then((cache) => cache.put(APP_SHELL, copy));
					}
					return response;
				})
				.catch(() => caches.match(APP_SHELL).then((response) => response ?? Response.error()))
		);
		return;
	}

	event.respondWith(
		caches.match(request).then((cached) => {
			if (cached) {
				return cached;
			}

			return fetch(request).then((response) => {
				if (
					response.ok &&
					(url.pathname.startsWith('/_app/') ||
						url.pathname.startsWith('/icons/') ||
						url.pathname === '/manifest.webmanifest')
				) {
					const copy = response.clone();
					caches.open(STATIC_CACHE).then((cache) => cache.put(request, copy));
				}
				return response;
			});
		})
	);
});
