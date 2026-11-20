<script lang="ts">
	import Spinner from "@/lib/Spinner.svelte";
	import { req } from "@/lib/util/api";
	import Poster from "@/lib/poster/Poster.svelte";
	import PageTitle from "@/lib/generic/PageTitle.svelte";
	import MediaTypeFilter from "@/lib/search/MediaTypeFilter.svelte";
	import infScroll from "@/lib/util/infScroll";
	import paginatedLoader, {
		PaginatedLoaderRunFnAction,
	} from "@/lib/util/paginatedLoader.svelte";
	import {
		DiscoverFilter,
		MediaTypeE,
		SearchType,
		type DiscoverRequest,
		type Media,
		type PaginationResponse,
	} from "@/types";
	import { page } from "$app/state";
	import { afterNavigate, goto } from "$app/navigation";
	import { onDestroy, onMount } from "svelte";
	import PosterList from "@/lib/poster/PosterList.svelte";
	import Error from "@/lib/Error.svelte";
	import PersonPoster from "@/lib/poster/PersonPoster.svelte";
	import FilterDropDown from "./FilterDropDown.svelte";
	import RegionDropDown from "@/lib/RegionDropDown.svelte";
	import { resolve } from "$app/paths";

	const scroll = infScroll({ callback: onScrollToBottom });
	const dataLoader = paginatedLoader<Media, undefined>(load);

	// Everything below is read from the url, so back/reload/shared links keep
	// the same results. Change them with `navigate`.
	let discoverFilter: DiscoverFilter = $derived.by(() => {
		const f = page.url.searchParams.get("filter");
		return (Object.values(DiscoverFilter) as string[]).includes(f ?? "")
			? (f as DiscoverFilter)
			: DiscoverFilter.trending;
	});
	let discoverType: SearchType | undefined = $derived.by(() => {
		const t = page.url.searchParams.get("type");
		if (t) {
			return t as SearchType;
		}
		return SearchType.multi;
	});
	// Year/country filters: only for popular movies (year + country) and
	// popular shows (year only).
	const currentYear = new Date().getFullYear();
	const parseYear = (v: string | null | undefined) => {
		const n = Number(v);
		return v && Number.isInteger(n) && n >= 1870 && n <= 2200 ? n : undefined;
	};
	let filtersSupported = $derived(
		discoverFilter === DiscoverFilter.popular &&
			(discoverType === SearchType.movie || discoverType === SearchType.show),
	);
	let countrySupported = $derived(
		filtersSupported && discoverType === SearchType.movie,
	);
	let yearFrom = $derived(
		filtersSupported
			? parseYear(page.url.searchParams.get("yearFrom"))
			: undefined,
	);
	let yearTo = $derived(
		filtersSupported
			? parseYear(page.url.searchParams.get("yearTo"))
			: undefined,
	);
	let country = $derived.by(() => {
		const c = (page.url.searchParams.get("country") ?? "").toUpperCase();
		return countrySupported && /^[A-Z]{2}$/.test(c) ? c : undefined;
	});

	let nextLoadParams: DiscoverRequest = $derived({
		page: dataLoader.state.page + 1,
		type: discoverType,
		filter: discoverFilter,
		yearFrom,
		yearTo,
		country,
	});

	interface DiscoverParams {
		type?: SearchType;
		filter?: DiscoverFilter;
		yearFrom?: number;
		yearTo?: number;
		country?: string;
	}

	/** Current url params (to change one of them with `navigate`). */
	const current = (): DiscoverParams => ({
		type: discoverType,
		filter: discoverFilter,
		yearFrom,
		yearTo,
		country,
	});

	/**
	 * Go to discover with these params (defaults are left out of the url).
	 * The afterNavigate hook then loads the new results.
	 */
	function navigate(p: DiscoverParams) {
		const parts: string[] = [];
		if (p.type && p.type !== SearchType.multi) parts.push(`type=${p.type}`);
		if (p.filter && p.filter !== DiscoverFilter.trending) {
			parts.push(`filter=${p.filter}`);
		}
		if (p.yearFrom) parts.push(`yearFrom=${p.yearFrom}`);
		if (p.yearTo) parts.push(`yearTo=${p.yearTo}`);
		if (p.country) parts.push(`country=${encodeURIComponent(p.country)}`);
		goto(resolve(`/discover?${parts.join("&")}`), {
			keepFocus: true,
			noScroll: true,
		});
	}

	function setYear(which: "from" | "to", raw: string) {
		let from = which === "from" ? parseYear(raw) : yearFrom;
		let to = which === "to" ? parseYear(raw) : yearTo;
		if (from && to && from > to) {
			[from, to] = [to, from];
		}
		navigate({ ...current(), yearFrom: from, yearTo: to });
	}

	async function load(signal: AbortSignal) {
		console.debug("load: loadParams:", nextLoadParams);
		if (nextLoadParams.page === dataLoader.state.page) {
			console.warn("load: Already on this page, not loading it again!");
			return;
		}
		const r = await req.get<PaginationResponse<Media, undefined>>(`/discover`, {
			params: nextLoadParams,
			signal,
		});
		scroll.dataLoaded();
		return r;
	}

	async function onScrollToBottom() {
		// If an error is being shown, no more infinite scroll.
		if (dataLoader.state.reqLoadError) {
			return;
		}
		dataLoader.runFn();
	}

	function setActiveDiscoverType(to: SearchType | undefined) {
		console.debug("setActiveDiscoverType: to:", to);
		// Filter (and year/country) are reset when changing type, since the
		// new type may not support them.
		navigate({ type: !to || discoverType === to ? undefined : to });
	}

	onMount(() => {
		dataLoader.runFn(PaginatedLoaderRunFnAction.Reset);
	});

	afterNavigate(() => {
		console.log(
			"afterNavigate: Query changed, performing request.",
			"searchParams:",
			page.url.searchParams,
		);
		dataLoader.abortReq("navigated away");
		dataLoader.runFn(PaginatedLoaderRunFnAction.Reset);
	});

	onDestroy(() => {
		console.debug("DISCOVER PAGE DESTROYED");
		scroll.destroy();
		dataLoader.abortReq("page destroyed");
	});
</script>

<svelte:head>
	<title>Discover Content</title>
</svelte:head>

<div class="content">
	<div class="inner">
		<PageTitle title="Discover">
			<div class="pagetitle-mediatypefilter">
				<MediaTypeFilter
					active={discoverType}
					disabled={false}
					onChange={(nowActive) => {
						setActiveDiscoverType(nowActive as SearchType | undefined);
					}}
				/>
			</div>
			<div class="pagetitle-filterdropdown">
				<FilterDropDown
					{discoverType}
					active={discoverFilter}
					onChange={(f) => {
						console.log("Discover FilterDropDown Selected Change", f);
						// Year/country filters reset with the main filter.
						navigate({ type: discoverType, filter: f });
					}}
				/>
			</div>
		</PageTitle>

		{#if filtersSupported}
			<div class="filters" role="group" aria-label="Filters">
				<div class="years">
					<span>
						{country
							? "In theatres"
							: discoverType === SearchType.show
								? "First aired"
								: "Released"}
					</span>
					<input
						type="number"
						placeholder="From"
						aria-label="From year"
						min="1870"
						max={currentYear + 5}
						value={yearFrom ?? ""}
						onchange={(e) => setYear("from", e.currentTarget.value)}
					/>
					<span>–</span>
					<input
						type="number"
						placeholder="To"
						aria-label="To year"
						min="1870"
						max={currentYear + 5}
						value={yearTo ?? ""}
						onchange={(e) => setYear("to", e.currentTarget.value)}
					/>
				</div>
				{#if countrySupported}
					<div class="country">
						<span>in</span>
						{#key country}
							<RegionDropDown
								selectedCountry={country ?? ""}
								onChange={(c) => navigate({ ...current(), country: c })}
							/>
						{/key}
					</div>
				{/if}
				{#if yearFrom || yearTo || country}
					<button
						class="plain clear"
						onclick={() =>
							navigate({ type: discoverType, filter: discoverFilter })}
					>
						Clear filters
					</button>
				{/if}
			</div>
			{#if country}
				<p class="filters-hint">
					Films with a theatrical release in that country during these years
					(re-releases included).
				</p>
			{/if}
		{/if}

		<PosterList>
			{#if dataLoader.state.data?.length > 0}
				{#each dataLoader.state.data as w, i (`${i}-${w.type}`)}
					{#if w.type === MediaTypeE.tmdbPerson}
						<PersonPoster
							id={w.ids.tmdb}
							name={w.name}
							path={w.extPosterPath}
						/>
					{:else if w.type === MediaTypeE.tmdbMovie || w.type === MediaTypeE.tmdbShow || w.type === MediaTypeE.igdbGame}
						<Poster
							media={w}
							bind:watched={dataLoader.state.data[i].watched}
							fluidSize
						/>
					{/if}
				{/each}
			{:else if !dataLoader.state.reqLoading && !dataLoader.state.reqLoadError}
				<!-- If request is running or we have an error, no point in showing 'no results' message. -->
				<h2 class="norm" title="Hovering over me doesn't change the facts ;(">
					No Results!
				</h2>
			{/if}
		</PosterList>

		{#if dataLoader.state.reqLoading}
			<div style="margin-bottom: 60px;">
				<Spinner />
			</div>
		{/if}

		{#if dataLoader.state.reqLoadError}
			<div style="margin-bottom: 60px;">
				<Error
					pretty="Failed to load results!"
					error={dataLoader.state.reqLoadError}
					onRetry={() => {
						dataLoader.state.reqLoadError = undefined;
						dataLoader.runFn(PaginatedLoaderRunFnAction.ResetIfOnFirstOrNoPage);
					}}
				/>
			</div>
		{/if}
	</div>
</div>

<style lang="scss">
	.pagetitle-mediatypefilter {
		@media screen and (max-width: 745px) {
			width: 100%;
			order: 2;
		}
	}

	.pagetitle-filterdropdown {
		margin-left: auto;
	}

	.filters {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 10px 18px;
		/* Same side margins as the PageTitle row above. */
		margin: 0 15px 6px 15px;
		font-size: 14px;

		.years,
		.country {
			display: flex;
			align-items: center;
			gap: 8px;
		}

		input {
			width: 90px;
			padding: 6px 8px;
		}

		.country :global(> div) {
			min-width: 200px;
		}

		.clear {
			width: auto;
			color: $text-color-accent;
			text-decoration: underline;
		}
	}

	.filters-hint {
		margin: 0 15px 10px 15px;
		font-size: 13px;
		color: $text-color-accent;
	}

	.content {
		display: flex;
		width: 100%;
		justify-content: center;

		.inner {
			width: 100%;
			max-width: 1200px;
		}
	}
</style>
