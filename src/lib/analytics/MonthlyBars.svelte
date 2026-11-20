<!--
	Single series column chart, one column per month.
	Always fits the available width (no scrolling), so with long periods
	columns get thin and axis labels are thinned out to never overlap.
-->
<script lang="ts">
	import { formatMonth, monthAbbr, plural } from "./util";

	interface Props {
		/** Chart title, also names the series. */
		title: string;
		months: { month: string; value: number }[];
		/** Unit used in tooltips/labels, eg ["film", "films"]. */
		unit: [string, string];
		/** CSS color of the bars (a --viz-series-* var). */
		color: string;
	}

	let { title, months, unit, color }: Props = $props();

	const LEFT = 40;
	const TOP = 18;
	const PLOT_H = 140;
	const BOTTOM = 34;
	const HEIGHT = TOP + PLOT_H + BOTTOM;
	const MAX_BAR = 24;
	/** Min px between the starts of two month labels ("Sep"). */
	const MONTH_LABEL_SPACE = 30;
	/** Min px between the starts of two year labels ("2026"). */
	const YEAR_LABEL_SPACE = 40;
	/** Tooltip half width used to keep it inside the chart. */
	const TIP_HALF = 55;

	let plotW = $state(600);
	let hovered: number | undefined = $state();

	let n = $derived(months.length);
	let max = $derived(months.reduce((m, v) => Math.max(m, v.value), 0));
	let maxIdx = $derived(months.findIndex((m) => m.value === max));
	let slot = $derived(plotW / Math.max(n, 1));
	// Keep a gap between columns, even when they get very thin.
	let barW = $derived(
		Math.max(1, Math.min(MAX_BAR, slot * 0.65, slot - (slot > 4 ? 2 : 1))),
	);

	/** Clean integer ticks from 0 to a nice max. */
	let ticks = $derived.by(() => {
		if (max <= 4)
			return Array.from({ length: Math.max(max, 1) + 1 }, (_, i) => i);
		const raw = max / 4;
		const mag = 10 ** Math.floor(Math.log10(raw));
		const step = [1, 2, 5, 10].map((s) => s * mag).find((s) => s >= raw) ?? raw;
		const top = Math.ceil(max / step) * step;
		const t: number[] = [];
		for (let v = 0; v <= top; v += step) t.push(v);
		return t;
	});
	let yMax = $derived(ticks[ticks.length - 1] || 1);

	const y = (v: number) => TOP + PLOT_H - (v / yMax) * PLOT_H;
	const monthNum = (i: number) => Number(months[i].month.split("-")[1]);
	const yearOf = (i: number) => Number(months[i].month.split("-")[0]);

	interface XLabel {
		i: number;
		month?: string;
		year?: string;
	}

	/**
	 * Which x labels to draw, picked so they fit the available width.
	 * Short periods label months (with the year under January and the first
	 * column), long periods only label years.
	 */
	let labels: XLabel[] = $derived.by(() => {
		const out: XLabel[] = [];
		const monthsEvery = [1, 2, 3, 6].find((m) => m * slot >= MONTH_LABEL_SPACE);
		if (monthsEvery) {
			for (let i = 0; i < n; i++) {
				const mo = monthNum(i);
				const showMonth = (mo - 1) % monthsEvery === 0 || i === 0;
				const showYear = mo === 1 || i === 0;
				if (showMonth || showYear) {
					out.push({
						i,
						month: showMonth ? monthAbbr[mo - 1] : undefined,
						year: showYear ? String(yearOf(i)) : undefined,
					});
				}
			}
			// Don't let the first column's labels collide with the next ones.
			const nextYear = out.find((l, idx) => idx > 0 && l.year);
			if (nextYear && nextYear.i * slot < YEAR_LABEL_SPACE) {
				out[0].year = undefined;
			}
			const nextMonth = out.find((l, idx) => idx > 0 && l.month);
			if (nextMonth && nextMonth.i * slot < MONTH_LABEL_SPACE) {
				out[0].month = undefined;
			}
			return out;
		}
		// Years only, every `k` years so they don't overlap.
		const k =
			[1, 2, 5, 10, 20, 50].find((k) => k * 12 * slot >= YEAR_LABEL_SPACE) ??
			100;
		for (let i = 0; i < n; i++) {
			if (monthNum(i) === 1 && yearOf(i) % k === 0) {
				out.push({ i, year: String(yearOf(i)) });
			}
		}
		return out;
	});
	let anyMonthLabel = $derived(labels.some((l) => l.month));

	/** Column path: up to 4px rounded top, square at the baseline. */
	function barPath(x: number, v: number) {
		const top = y(v);
		const h = TOP + PLOT_H - top;
		const r = Math.min(4, h, barW / 2);
		return `M${x},${top + h} V${top + r} Q${x},${top} ${x + r},${top} H${x + barW - r} Q${x + barW},${top} ${x + barW},${top + r} V${top + h} Z`;
	}

	const barX = (i: number) => i * slot + (slot - barW) / 2;
	const clampX = (x: number, pad: number) =>
		Math.min(Math.max(x, pad), Math.max(pad, plotW - pad));

	/** The pointer selects the month column it is in (thin bars are hard to hit). */
	function onPointerMove(e: PointerEvent) {
		const rect = (e.currentTarget as SVGRectElement).getBoundingClientRect();
		const i = Math.floor((e.clientX - rect.left) / slot);
		hovered = i >= 0 && i < n ? i : undefined;
	}
</script>

<figure>
	<figcaption>{title}</figcaption>
	{#if max === 0}
		<p class="empty">Nothing in this period.</p>
	{:else}
		<div class="chart">
			<svg class="y-axis" width={LEFT} height={HEIGHT} aria-hidden="true">
				{#each ticks as t (t)}
					<text class="axis tick" x={LEFT - 6} y={y(t) + 4}
						>{t.toLocaleString()}</text
					>
				{/each}
			</svg>
			<div class="plot" bind:clientWidth={plotW}>
				<svg
					width={plotW}
					height={HEIGHT}
					role="img"
					aria-label="{title} per month, up to {plural(max, unit[0], unit[1])}"
				>
					{#each ticks as t (t)}
						<line class="grid" x1={0} x2={plotW} y1={y(t)} y2={y(t)} />
					{/each}
					{#each months as m, i (m.month)}
						{#if m.value > 0}
							<path
								d={barPath(barX(i), m.value)}
								fill={color}
								class:dim={hovered !== undefined && hovered !== i}
							/>
						{/if}
					{/each}
					{#if maxIdx >= 0}
						<text
							class="value"
							x={clampX(barX(maxIdx) + barW / 2, 10)}
							y={y(max) - 5}
						>
							{max.toLocaleString()}
						</text>
					{/if}
					{#each labels as l (l.i)}
						{@const x = clampX(barX(l.i) + barW / 2, 12)}
						{#if l.month}
							<text class="axis" {x} y={TOP + PLOT_H + 15}>{l.month}</text>
						{/if}
						{#if l.year}
							<text
								class="axis year"
								{x}
								y={TOP + PLOT_H + (anyMonthLabel ? 29 : 15)}
							>
								{l.year}
							</text>
						{/if}
					{/each}
					<!-- One hit area for the whole plot, mapped to the column under the pointer. -->
					<rect
						class="hit"
						x={0}
						y={TOP}
						width={plotW}
						height={PLOT_H}
						role="presentation"
						onpointermove={onPointerMove}
						onpointerleave={() => (hovered = undefined)}
					/>
				</svg>
				{#if hovered !== undefined}
					{@const m = months[hovered]}
					<div
						class="tip"
						style:left="{clampX(hovered * slot + slot / 2, TIP_HALF)}px"
						style:top="{Math.min(y(m.value), TOP + PLOT_H - 10) - 8}px"
					>
						<strong>{plural(m.value, unit[0], unit[1])}</strong>
						<span>{formatMonth(m.month)}</span>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</figure>

<style lang="scss">
	figure {
		margin: 0;
		display: flex;
		flex-flow: column;
		gap: 8px;
	}

	figcaption {
		font-weight: bold;
		font-size: 15px;
	}

	.empty {
		color: $text-color-accent;
		font-size: 14px;
		padding: 20px 0;
	}

	.chart {
		display: flex;
		align-items: flex-start;
	}

	.y-axis {
		flex-shrink: 0;
	}

	.plot {
		position: relative;
		flex: 1;
		min-width: 0;
	}

	svg {
		display: block;
		overflow: visible;
	}

	.grid {
		stroke: var(--viz-grid);
		stroke-width: 1px;
		shape-rendering: crispEdges;
	}

	.axis {
		font-size: 11px;
		fill: $text-color-accent;
		font-variant-numeric: tabular-nums;
		text-anchor: middle;
		user-select: none;

		&.tick {
			text-anchor: end;
		}

		&.year {
			font-weight: bold;
		}
	}

	.value {
		font-size: 12px;
		font-weight: bold;
		fill: $text-color;
		text-anchor: middle;
	}

	path {
		transition: opacity 120ms ease;

		&.dim {
			opacity: 0.45;
		}
	}

	.hit {
		fill: transparent;
	}

	.tip {
		position: absolute;
		z-index: 5;
		display: flex;
		flex-flow: column;
		pointer-events: none;
		transform: translate(-50%, -100%);
		white-space: nowrap;
		padding: 6px 9px;
		font-size: 13px;
		background-color: $bg-color;
		color: $text-color;
		border: 1px solid $bg-color-accent;
		border-radius: 6px;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);

		span {
			color: $text-color-accent;
			font-size: 12px;
		}
	}
</style>
