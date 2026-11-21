<script lang="ts">
	import { goto, replaceState } from "$app/navigation";
	import { page } from "$app/state";
	import { resolve } from "$app/paths";
	import { onMount } from "svelte";
	import { type AuthResponse } from "@/types";
	import { noAuthReq } from "@/lib/util/api";
	import { notify } from "@/lib/util/notify";
	import { ReqerError } from "@/lib/util/fetch";
	import Spinner from "@/lib/Spinner.svelte";

	let error: string | undefined = $state();

	onMount(async () => {
		const params = page.url.searchParams;
		const code = params.get("code");
		const state = params.get("state");
		const providerErr = params.get("error");

		if (providerErr) {
			console.error(
				"oidc_callback: Provider returned error:",
				providerErr,
				params.get("error_description"),
			);
			error = params.get("error_description") || providerErr;
			return;
		}
		if (!code || !state) {
			error = "Missing code or state from provider";
			return;
		}
		try {
			const resp = await noAuthReq.post<AuthResponse>("/auth/oidc/callback", {
				code,
				state,
			});
			if (!resp?.token) {
				throw new Error("no token returned");
			}
			console.log("Received token... logging in.");
			localStorage.setItem("token", resp.token);
			// Replace this page in history, code/state are single use.
			goto(resolve("/"), { replaceState: true });
			notify({ text: "Welcome!", type: "success" });
		} catch (err) {
			console.error("oidc_callback: Login failed!", err);
			// Remove the used code/state from the url bar & history.
			// Only done after awaiting the request: on first page load, onMount
			// runs before SvelteKit's router is initialized, and shallow routing
			// then fails (it throws in dev, and crashes on `root.$set` in prod).
			replaceState(resolve("/oidc_callback"), {});
			error = ReqerError.getMsg(err, "Login failed");
		}
	});
</script>

<div>
	{#if error}
		<span class="error">{error}!</span>
		<a href={resolve("/login")}>Back to login</a>
	{:else}
		<Spinner />
		<p>Logging you in...</p>
	{/if}
</div>

<style lang="scss">
	div {
		display: flex;
		flex-flow: column;
		align-items: center;
		gap: 15px;
		margin: 0 35px;
	}

	.error {
		display: flex;
		justify-content: center;
		width: 100%;
		max-width: 400px;
		padding: 10px;
		background-color: rgb(221, 48, 48);
		text-transform: capitalize;
		color: white;
	}
</style>
