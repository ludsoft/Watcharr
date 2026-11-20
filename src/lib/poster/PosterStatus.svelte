<script lang="ts">
	import type { FinishedAtReleaseOption, WatchedStatus } from "@/types";
	import Icon from "../Icon.svelte";
	import tooltip from "../actions/tooltip";
	import { toUnderstandableStatus, watchedStatuses } from "../util/helpers";

	interface Props {
		status?: WatchedStatus | undefined;
		handleStatusClick: (status: WatchedStatus | "DELETE") => void;
		direction?: "top" | "bot";
		minimal?: boolean;
		btnTooltip?: string;
		disableInteraction?: boolean;
		isForGame?: boolean;
		/** Show the "Finished at release date" option (after Finished). */
		finishedAtRelease?: FinishedAtReleaseOption;
	}

	let {
		status = undefined,
		handleStatusClick,
		direction = "top",
		minimal = false,
		btnTooltip = "",
		disableInteraction = false,
		isForGame = false,
		finishedAtRelease = undefined,
	}: Props = $props();

	let statusesShown = $state(false);
</script>

<button
	class={["status", disableInteraction ? "interaction-disabled" : ""].join(" ")}
	class:minimal
	onclick={(ev) => {
		ev.stopPropagation();
		statusesShown = !statusesShown;
	}}
	onmouseleave={() => {
		statusesShown = false;
	}}
	use:tooltip={{
		text: btnTooltip,
		pos: "top",
		condition: !!btnTooltip && !statusesShown,
	}}
>
	{#if status}
		<Icon i={watchedStatuses[status]} />
	{:else}
		<span class={["no-icon", minimal ? "minimal" : ""].join(" ")}>+</span>
	{/if}
	<div
		class={[
			statusesShown ? "shown" : "",
			"small-scrollbar",
			status ? "has-status" : "",
			direction,
		].join(" ")}
	>
		{#each Object.entries(watchedStatuses) as [statusName, icon] (statusName)}
			<!-- svelte-ignore node_invalid_placement_ssr -->
			<button
				class="plain{status && status !== statusName ? ' not-active' : ''}"
				onclick={() => handleStatusClick(statusName as WatchedStatus)}
				use:tooltip={{
					text: toUnderstandableStatus(statusName as WatchedStatus, isForGame),
				}}
			>
				<Icon i={icon} />
			</button>
			{#if statusName === "FINISHED" && finishedAtRelease}
				<!-- svelte-ignore node_invalid_placement_ssr -->
				<button
					class="plain"
					class:not-active={!!status}
					class:disabled={!!finishedAtRelease.disabledReason}
					aria-disabled={!!finishedAtRelease.disabledReason}
					onclick={(ev) => {
						if (finishedAtRelease.disabledReason) {
							ev.stopPropagation();
							return;
						}
						finishedAtRelease.onClick();
					}}
					use:tooltip={{
						text: finishedAtRelease.disabledReason
							? `Finished at release date (${finishedAtRelease.disabledReason})`
							: "Finished at release date",
					}}
				>
					<Icon i="calendar-check" />
				</button>
			{/if}
		{/each}
		{#if status}
			<!-- svelte-ignore node_invalid_placement_ssr -->
			<button
				class="plain not-active"
				onclick={() => handleStatusClick("DELETE")}
				use:tooltip={{ text: "Delete" }}
			>
				<Icon i="trash" />
			</button>
		{/if}
	</div>
</button>

<style lang="scss">
	button.status {
		padding: 3px;
		position: relative;
		font-family: "Rampart One";
		height: 100%;
		width: 40%;

		&.minimal {
			width: 100%;

			div {
				height: unset;
				overflow: unset;

				/* Menu grows with its content here, keep icons full width. */
				button {
					flex: none;

					& :global(svg) {
						width: 100%;
						height: auto;
					}
				}
			}
		}

		&.interaction-disabled {
			pointer-events: none;
			cursor: default;
			background-color: transparent;
			border: unset;
			fill: white;
			color: white;

			span {
				color: white !important;
			}
		}

		.no-icon {
			color: $text-color;
			font-size: 30px;
			height: 52px;

			&.minimal {
				height: 30px;
				line-height: 22px;
			}
		}

		&:hover .no-icon,
		&:focus-visible .no-icon {
			color: $bg-color;
			fill: $bg-color;
		}

		div {
			display: flex;
			flex-flow: column;
			position: absolute;
			width: 100%;
			height: 200px;
			background-color: $bg-color;
			top: calc(-100% - 170px);
			list-style: none;
			border-radius: 4px 4px 0 0;
			/* Options share the height (see buttons below), nothing to scroll. */
			overflow: hidden;
			z-index: 40;
			box-shadow: 0px 0px 1px #000;

			&:not(.shown) {
				display: none;
			}

			&.bot {
				top: calc(100% + 2px);
				border-radius: 0 0 4px 4px;
			}

			button {
				/* Every option gets an equal share of the menu height and its
				icon is scaled to fit it, so all options fit without scrolling. */
				flex: 1 1 0;
				min-height: 0;
				display: flex;
				align-items: center;
				justify-content: center;
				padding: 2px 0;
				width: 100%;
				color: $text-color;
				fill: $text-color;
				-webkit-text-stroke: 0.5px $text-color;

				& :global(svg) {
					height: 100%;
					width: auto;
					max-width: 100%;
					padding: 0 2px;
				}

				&:hover,
				&:focus-visible {
					background-color: rgb(100, 100, 100, 0.5);
				}

				/* Not usable right now, tooltip says why. */
				&.disabled {
					opacity: 0.35;
					cursor: not-allowed;

					&:hover {
						background-color: transparent;
					}
				}
			}

			&.has-status :global(svg) {
				padding: 0 4.5px;
			}
		}
	}
</style>
