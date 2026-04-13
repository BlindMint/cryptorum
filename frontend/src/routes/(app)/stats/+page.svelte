<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import { browser } from '$app/environment';

	type StatsSection = 'library' | 'reading' | 'discover';

	type ChartCount = {
		label: string;
		count: number;
	};

	let loading = $state(true);
	let stats = $state<any>(null);
	let activeSection = $state<StatsSection>('library');
	let chartRegistered = false;
	let renderVersion = 0;
	let charts: any[] = [];

	let formatChartCanvas = $state<HTMLCanvasElement | null>(null);
	let genreChartCanvas = $state<HTMLCanvasElement | null>(null);
	let authorChartCanvas = $state<HTMLCanvasElement | null>(null);
	let languageChartCanvas = $state<HTMLCanvasElement | null>(null);
	let yearChartCanvas = $state<HTMLCanvasElement | null>(null);
	let pageBucketsChartCanvas = $state<HTMLCanvasElement | null>(null);
	let readingActivityChartCanvas = $state<HTMLCanvasElement | null>(null);
	let sessionBucketsChartCanvas = $state<HTMLCanvasElement | null>(null);
	let readingTrendChartCanvas = $state<HTMLCanvasElement | null>(null);

	onMount(async () => {
		if (!browser) return;
		await fetchStats();
		loading = false;
	});

	$effect(() => {
		if (!browser || loading || !stats) return;
		const section = activeSection;
		void renderCharts(section);
	});

	onDestroy(() => {
		destroyCharts();
	});

	async function fetchStats() {
		try {
			const res = await fetch('/api/stats');
			if (res.ok) {
				stats = await res.json();
			}
		} catch (error) {
			console.error('Failed to fetch stats:', error);
		}
	}

	function destroyCharts() {
		for (const chart of charts) {
			try {
				chart.destroy();
			} catch {
				// Ignore chart teardown failures.
			}
		}
		charts = [];
	}

	async function renderCharts(section: StatsSection) {
		const version = ++renderVersion;
		await tick();
		if (version !== renderVersion || !stats) return;

		destroyCharts();

		const { Chart, registerables } = await import('chart.js');
		if (!chartRegistered) {
			Chart.register(...registerables);
			chartRegistered = true;
		}

		const rootStyles = getComputedStyle(document.documentElement);
		const textColor = rootStyles.getPropertyValue('--color-surface-text').trim() || '#e5e7eb';
		const mutedColor = rootStyles.getPropertyValue('--color-surface-text-muted').trim() || '#94a3b8';
		const borderColor = rootStyles.getPropertyValue('--color-surface-border').trim() || 'rgba(255,255,255,0.08)';

		const commonOptions = {
			responsive: true,
			maintainAspectRatio: false,
			plugins: {
				legend: {
					labels: {
						color: textColor
					}
				}
			}
		};

		const addChart = (canvas: HTMLCanvasElement | null, config: any) => {
			if (!canvas) return;
			const ctx = canvas.getContext('2d');
			if (!ctx) return;
			charts.push(new Chart(ctx, config));
		};

		if (section === 'library') {
			const formatItems = [
				['EPUB', stats.books_by_format?.epub ?? 0, 'rgba(245, 158, 11, 0.82)'],
				['PDF', stats.books_by_format?.pdf ?? 0, 'rgba(59, 130, 246, 0.82)'],
				['Comics', stats.books_by_format?.cbx ?? 0, 'rgba(236, 72, 153, 0.82)'],
				['Audio', stats.books_by_format?.audio ?? 0, 'rgba(34, 197, 94, 0.82)'],
				['Other', stats.books_by_format?.other ?? 0, 'rgba(148, 163, 184, 0.82)']
			].filter(([, count]) => count > 0);

			addChart(formatChartCanvas, {
				type: 'doughnut',
				data: {
					labels: formatItems.map(([label]) => label),
					datasets: [
						{
							data: formatItems.map(([, count]) => count),
							backgroundColor: formatItems.map(([, , color]) => color),
							borderColor: 'rgba(255,255,255,0.12)',
							borderWidth: 2
						}
					]
				},
				options: {
					...commonOptions,
					cutout: '65%',
					plugins: {
						...commonOptions.plugins,
						title: {
							display: true,
							text: 'Books by Format',
							color: textColor
						}
					}
				}
			});

			addChart(genreChartCanvas, {
				type: 'doughnut',
				data: {
					labels: (stats.genre_distribution ?? []).map((item: any) => item.name),
					datasets: [
						{
							data: (stats.genre_distribution ?? []).map((item: any) => item.count),
							backgroundColor: [
								'rgba(245, 158, 11, 0.82)',
								'rgba(239, 68, 68, 0.82)',
								'rgba(59, 130, 246, 0.82)',
								'rgba(34, 197, 94, 0.82)',
								'rgba(168, 85, 247, 0.82)',
								'rgba(6, 182, 212, 0.82)',
								'rgba(236, 72, 153, 0.82)',
								'rgba(249, 115, 22, 0.82)'
							].slice(0, (stats.genre_distribution ?? []).length),
							borderColor: 'rgba(255,255,255,0.12)',
							borderWidth: 2
						}
					]
				},
				options: {
					...commonOptions,
					cutout: '60%',
					plugins: {
						...commonOptions.plugins,
						title: {
							display: true,
							text: 'Top Genres',
							color: textColor
						}
					}
				}
			});

			addChart(authorChartCanvas, {
				type: 'bar',
				data: {
					labels: (stats.author_distribution ?? []).map((item: any) => item.name),
					datasets: [
						{
							label: 'Books',
							data: (stats.author_distribution ?? []).map((item: any) => item.count),
							backgroundColor: 'rgba(168, 85, 247, 0.65)',
							borderColor: 'rgb(168, 85, 247)',
							borderWidth: 1,
							borderRadius: 8
						}
					]
				},
				options: {
					...commonOptions,
					indexAxis: 'y',
					plugins: {
						...commonOptions.plugins,
						title: {
							display: true,
							text: 'Top Authors',
							color: textColor
						},
						legend: {
							display: false
						}
					},
					scales: {
						x: {
							beginAtZero: true,
							ticks: { color: mutedColor },
							grid: { color: borderColor }
						},
						y: {
							ticks: { color: mutedColor },
							grid: { display: false }
						}
					}
				}
			});

			addChart(languageChartCanvas, {
				type: 'bar',
				data: {
					labels: (stats.language_distribution ?? []).map((item: any) => item.label),
					datasets: [
						{
							label: 'Books',
							data: (stats.language_distribution ?? []).map((item: any) => item.count),
							backgroundColor: 'rgba(6, 182, 212, 0.65)',
							borderColor: 'rgb(6, 182, 212)',
							borderWidth: 1,
							borderRadius: 8
						}
					]
				},
				options: {
					...commonOptions,
					indexAxis: 'y',
					plugins: {
						...commonOptions.plugins,
						title: {
							display: true,
							text: 'Languages',
							color: textColor
						},
						legend: {
							display: false
						}
					},
					scales: {
						x: {
							beginAtZero: true,
							ticks: { color: mutedColor },
							grid: { color: borderColor }
						},
						y: {
							ticks: { color: mutedColor },
							grid: { display: false }
						}
					}
				}
			});

			addChart(yearChartCanvas, {
				type: 'line',
				data: {
					labels: (stats.pub_year_timeline ?? []).map((item: any) => String(item.year)),
					datasets: [
						{
							label: 'Books Published',
							data: (stats.pub_year_timeline ?? []).map((item: any) => item.count),
							borderColor: 'rgb(34, 197, 94)',
							backgroundColor: 'rgba(34, 197, 94, 0.15)',
							fill: true,
							tension: 0.32,
							pointRadius: 2
						}
					]
				},
				options: {
					...commonOptions,
					plugins: {
						...commonOptions.plugins,
						title: {
							display: true,
							text: 'Publication Year Timeline',
							color: textColor
						},
						legend: {
							display: false
						}
					},
					scales: {
						x: {
							ticks: { color: mutedColor },
							grid: { color: borderColor }
						},
						y: {
							beginAtZero: true,
							ticks: { color: mutedColor },
							grid: { color: borderColor }
						}
					}
				}
			});

			addChart(pageBucketsChartCanvas, {
				type: 'bar',
				data: {
					labels: (stats.page_count_buckets ?? []).map((item: any) => item.label),
					datasets: [
						{
							label: 'Books',
							data: (stats.page_count_buckets ?? []).map((item: any) => item.count),
							backgroundColor: 'rgba(245, 158, 11, 0.72)',
							borderColor: 'rgb(245, 158, 11)',
							borderWidth: 1,
							borderRadius: 8
						}
					]
				},
				options: {
					...commonOptions,
					plugins: {
						...commonOptions.plugins,
						title: {
							display: true,
							text: 'Page Count Histogram',
							color: textColor
						},
						legend: {
							display: false
						},
						tooltip: {
							callbacks: {
								title: (items: any[]) => items?.[0]?.label || '',
								label: (context: any) => `${context.raw} books`
							}
						}
					},
					scales: {
						x: {
							ticks: { color: mutedColor },
							grid: { color: borderColor }
						},
						y: {
							beginAtZero: true,
							ticks: { color: mutedColor },
							grid: { color: borderColor }
						}
					}
				}
			});
		}

		if (section === 'reading') {
			addChart(readingActivityChartCanvas, {
				type: 'bar',
				data: {
					labels: (stats.reading_activity ?? []).map((item: any) => item.date),
					datasets: [
						{
							label: 'Sessions',
							data: (stats.reading_activity ?? []).map((item: any) => item.sessions),
							backgroundColor: 'rgba(59, 130, 246, 0.7)',
							borderRadius: 8,
							yAxisID: 'y'
						},
						{
							label: 'Minutes',
							data: (stats.reading_activity ?? []).map((item: any) => item.minutes),
							borderColor: 'rgb(34, 197, 94)',
							backgroundColor: 'rgba(34, 197, 94, 0.12)',
							tension: 0.3,
							type: 'line',
							fill: true,
							yAxisID: 'y1',
							pointRadius: 2
						}
					]
				},
				options: {
					...commonOptions,
					plugins: {
						...commonOptions.plugins,
						title: {
							display: true,
							text: 'Reading Activity (Last 7 Days)',
							color: textColor
						}
					},
					scales: {
						y: {
							beginAtZero: true,
							ticks: { color: mutedColor },
							grid: { color: borderColor }
						},
						y1: {
							beginAtZero: true,
							position: 'right',
							ticks: { color: mutedColor },
							grid: { drawOnChartArea: false }
						},
						x: {
							ticks: { color: mutedColor },
							grid: { color: borderColor }
						}
					}
				}
			});

			addChart(sessionBucketsChartCanvas, {
				type: 'bar',
				data: {
					labels: (stats.session_buckets ?? []).map((item: any) => item.label),
					datasets: [
						{
							label: 'Sessions',
							data: (stats.session_buckets ?? []).map((item: any) => item.count),
							backgroundColor: 'rgba(168, 85, 247, 0.65)',
							borderColor: 'rgb(168, 85, 247)',
							borderWidth: 1,
							borderRadius: 8
						}
					]
				},
				options: {
					...commonOptions,
					plugins: {
						...commonOptions.plugins,
						title: {
							display: true,
							text: 'Session Duration Buckets',
							color: textColor
						},
						legend: {
							display: false
						}
					},
					scales: {
						x: {
							ticks: { color: mutedColor },
							grid: { color: borderColor }
						},
						y: {
							beginAtZero: true,
							ticks: { color: mutedColor },
							grid: { color: borderColor }
						}
					}
				}
			});

			addChart(readingTrendChartCanvas, {
				type: 'line',
				data: {
					labels: (stats.reading_progress ?? []).map((item: any) => {
						const date = new Date(item.date);
						return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
					}),
					datasets: [
						{
							label: 'Sessions Started',
							data: (stats.reading_progress ?? []).map((item: any) => item.books),
							borderColor: 'rgb(245, 158, 11)',
							backgroundColor: 'rgba(245, 158, 11, 0.14)',
							fill: true,
							tension: 0.3,
							pointRadius: 2
						}
					]
				},
				options: {
					...commonOptions,
					plugins: {
						...commonOptions.plugins,
						title: {
							display: true,
							text: 'Reading Sessions by Day',
							color: textColor
						},
						legend: {
							display: false
						}
					},
					scales: {
						x: {
							ticks: { color: mutedColor },
							grid: { color: borderColor }
						},
						y: {
							beginAtZero: true,
							ticks: { color: mutedColor },
							grid: { color: borderColor }
						}
					}
				}
			});
		}
	}

	function formatNumber(value: number): string {
		return new Intl.NumberFormat('en-US').format(value || 0);
	}

	function formatPercent(value: number): string {
		return `${Math.round(value || 0)}%`;
	}

	function formatMinutes(value: number): string {
		const total = Math.max(0, Math.round(value || 0));
		if (total < 60) return `${total}m`;
		const hours = Math.floor(total / 60);
		const minutes = total % 60;
		return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
	}

	function formatLibraryAge(firstAddedAt: number): string {
		if (!firstAddedAt) return 'No collection history yet';
		const start = new Date(firstAddedAt * 1000);
		const now = new Date();
		let years = now.getFullYear() - start.getFullYear();
		let months = now.getMonth() - start.getMonth();
		if (months < 0) {
			years -= 1;
			months += 12;
		}
		if (years <= 0) {
			return `${months} month${months === 1 ? '' : 's'} collecting`;
		}
		return `${years} year${years === 1 ? '' : 's'}${months > 0 ? `, ${months} month${months === 1 ? '' : 's'}` : ''} collecting`;
	}

	function getTopCount(items: ChartCount[] | undefined): ChartCount | null {
		if (!items || items.length === 0) return null;
		return [...items].sort((a, b) => b.count - a.count)[0] ?? null;
	}

	function getTopGenre() {
		return getTopCount((stats?.genre_distribution ?? []).map((item: any) => ({ label: item.name, count: item.count })));
	}

	function getTopAuthor() {
		return getTopCount((stats?.author_distribution ?? []).map((item: any) => ({ label: item.name, count: item.count })));
	}

	function getTopFormat() {
		if (!stats?.books_by_format) return null;
		const entries = [
			{ label: 'EPUB', count: stats.books_by_format.epub || 0 },
			{ label: 'PDF', count: stats.books_by_format.pdf || 0 },
			{ label: 'Comics', count: stats.books_by_format.cbx || 0 },
			{ label: 'Audio', count: stats.books_by_format.audio || 0 },
			{ label: 'Other', count: stats.books_by_format.other || 0 }
		].filter((item) => item.count > 0);
		return getTopCount(entries);
	}

	function getPeakYear() {
		const years = (stats?.pub_year_timeline ?? []) as { year: number; count: number }[];
		return getTopCount(years.map((item) => ({ label: String(item.year), count: item.count })));
	}

	function buildDiscoverInsights() {
		if (!stats) return [];

		const insights = [
			{
				title: 'Library Depth',
				value: formatNumber(stats.total_books),
				description: `${formatNumber(stats.total_pages)} pages total across your library.`,
				accent: 'from-[var(--color-primary-500)]/20 to-[var(--color-primary-500)]/5'
			},
			{
				title: 'Reading Rhythm',
				value: formatMinutes(stats.average_session_minutes),
				description: `Average reading session length. Total tracked time: ${formatMinutes(stats.total_session_minutes)}.`,
				accent: 'from-emerald-500/20 to-emerald-500/5'
			},
			{
				title: 'Reading Streak',
				value: `${stats.current_reading_streak || 0} day${stats.current_reading_streak === 1 ? '' : 's'}`,
				description: 'Consecutive days with at least one reading session.',
				accent: 'from-amber-500/20 to-amber-500/5'
			},
			{
				title: 'Most Common Format',
				value: getTopFormat()?.label || 'N/A',
				description: `${formatNumber(getTopFormat()?.count || 0)} books in your primary format.`,
				accent: 'from-sky-500/20 to-sky-500/5'
			},
			{
				title: 'Peak Publication Year',
				value: getPeakYear()?.label || 'N/A',
				description: `${formatNumber(getPeakYear()?.count || 0)} books published in that year.`,
				accent: 'from-fuchsia-500/20 to-fuchsia-500/5'
			},
			{
				title: 'Top Genre',
				value: getTopGenre()?.label || 'N/A',
				description: `${formatNumber(getTopGenre()?.count || 0)} books share that genre path.`,
				accent: 'from-cyan-500/20 to-cyan-500/5'
			}
		];

		return insights;
	}
</script>

<div class="space-y-6">
	<div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
		<div>
			<h1 class="text-2xl font-bold text-[var(--color-surface-text)]">Statistics</h1>
			<p class="mt-1 max-w-2xl text-[var(--color-surface-text-muted)]">
				A collection overview that balances library shape, reading habits, and a few useful surprises.
			</p>
		</div>

		<div class="inline-flex rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-1 shadow-sm">
			{#each [
				['library', 'Library'],
				['reading', 'Reading'],
				['discover', 'Discover']
			] as [key, label]}
				<button
					type="button"
					onclick={() => (activeSection = key as StatsSection)}
					class="rounded-lg px-4 py-2 text-sm font-medium transition-colors {activeSection === key ? 'bg-[var(--color-primary-500)] text-white' : 'text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)]'}"
				>
					{label}
				</button>
			{/each}
		</div>
	</div>

	{#if loading}
		<div class="flex justify-center py-16">
			<div class="h-12 w-12 animate-spin rounded-full border-b-2 border-[var(--color-primary-500)]"></div>
		</div>
	{:else if stats}
		{#if activeSection === 'library'}
			<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Total Books</div>
					<div class="mt-2 text-3xl font-semibold text-[var(--color-surface-text)]">{formatNumber(stats.total_books)}</div>
					<div class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Items in the collection</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Pages</div>
					<div class="mt-2 text-3xl font-semibold text-[var(--color-surface-text)]">{formatNumber(stats.total_pages)}</div>
					<div class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Total pages across all books</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Series</div>
					<div class="mt-2 text-3xl font-semibold text-[var(--color-surface-text)]">{formatNumber(stats.total_series)}</div>
					<div class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Distinct series names</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Authors</div>
					<div class="mt-2 text-3xl font-semibold text-[var(--color-surface-text)]">{formatNumber(stats.total_authors)}</div>
					<div class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Tracked author entries</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Genres</div>
					<div class="mt-2 text-3xl font-semibold text-[var(--color-surface-text)]">{formatNumber(stats.total_genres)}</div>
					<div class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Unique genre paths</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Library Age</div>
					<div class="mt-2 text-3xl font-semibold text-[var(--color-surface-text)]">{formatLibraryAge(stats.library_first_added_at)}</div>
					<div class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Since the first imported book</div>
				</div>
			</div>

			<div class="grid gap-6 xl:grid-cols-2">
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5 xl:col-span-2">
					<div class="h-72">
						<canvas bind:this={yearChartCanvas}></canvas>
					</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="h-72">
						<canvas bind:this={formatChartCanvas}></canvas>
					</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="h-72">
						<canvas bind:this={genreChartCanvas}></canvas>
					</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="h-72">
						<canvas bind:this={authorChartCanvas}></canvas>
					</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="h-72">
						<canvas bind:this={languageChartCanvas}></canvas>
					</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5 xl:col-span-2">
					<div class="h-72">
						<canvas bind:this={pageBucketsChartCanvas}></canvas>
					</div>
					<div class="mt-3 text-sm text-[var(--color-surface-text-muted)]">
						{stats.page_count_missing || 0} books have no recorded page count and are excluded from the histogram.
					</div>
				</div>
			</div>
		{:else if activeSection === 'reading'}
			<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Currently Reading</div>
					<div class="mt-2 text-3xl font-semibold text-[var(--color-primary-400)]">{formatNumber(stats.reading)}</div>
					<div class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Books in progress</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Completed</div>
					<div class="mt-2 text-3xl font-semibold text-emerald-400">{formatNumber(stats.finished)}</div>
					<div class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Books marked finished</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Unread</div>
					<div class="mt-2 text-3xl font-semibold text-[var(--color-surface-text)]">{formatNumber(stats.unread)}</div>
					<div class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Books not yet opened</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Sessions This Week</div>
					<div class="mt-2 text-3xl font-semibold text-[var(--color-primary-400)]">{formatNumber(stats.sessions_this_week)}</div>
					<div class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Started in the last 7 days</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Total Reading Time</div>
					<div class="mt-2 text-3xl font-semibold text-[var(--color-surface-text)]">{formatMinutes(stats.total_session_minutes)}</div>
					<div class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Across all recorded sessions</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Current Streak</div>
					<div class="mt-2 text-3xl font-semibold text-amber-400">{formatNumber(stats.current_reading_streak)} days</div>
					<div class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Consecutive reading days</div>
				</div>
			</div>

			<div class="grid gap-6 xl:grid-cols-2">
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5 xl:col-span-2">
					<div class="h-72">
						<canvas bind:this={readingActivityChartCanvas}></canvas>
					</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="h-72">
						<canvas bind:this={sessionBucketsChartCanvas}></canvas>
					</div>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="h-72">
						<canvas bind:this={readingTrendChartCanvas}></canvas>
					</div>
				</div>
			</div>
		{:else}
			<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
				{#each buildDiscoverInsights() as insight}
					<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
						<div class="rounded-xl bg-gradient-to-br {insight.accent} p-4">
							<div class="text-xs font-medium uppercase tracking-[0.2em] text-[var(--color-surface-text-muted)]">{insight.title}</div>
							<div class="mt-2 text-2xl font-semibold text-[var(--color-surface-text)]">{insight.value}</div>
							<p class="mt-2 text-sm leading-6 text-[var(--color-surface-text-muted)]">{insight.description}</p>
						</div>
					</div>
				{/each}
			</div>

			<div class="mt-6 grid gap-4 md:grid-cols-3">
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Library Age</div>
					<div class="mt-2 text-xl font-semibold text-[var(--color-surface-text)]">{formatLibraryAge(stats.library_first_added_at)}</div>
					<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Age of the collection from first added book to now.</p>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Peak Publication Year</div>
					<div class="mt-2 text-xl font-semibold text-[var(--color-surface-text)]">{getPeakYear()?.label || 'N/A'}</div>
					<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Year with the highest number of books in your library.</p>
				</div>
				<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-5">
					<div class="text-xs font-medium uppercase tracking-[0.18em] text-[var(--color-surface-text-muted)]">Collection Shape</div>
					<div class="mt-2 text-xl font-semibold text-[var(--color-surface-text)]">{getTopFormat()?.label || 'N/A'}</div>
					<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">Your most common file type shapes the reading experience.</p>
				</div>
			</div>
		{/if}
	{:else}
		<div class="rounded-2xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-6 py-16 text-center">
			<p class="text-[var(--color-surface-text-muted)]">Failed to load statistics.</p>
		</div>
	{/if}
</div>
