<h1 align="center">Watcharr (ludsoft fork)</h1>
<p align="center"><img src="./static/logo-col.png" alt="logo" width="250" /></p>

<p align="center">
  <a href="https://github.com/ludsoft/Watcharr/pkgs/container/watcharr"><img src="https://img.shields.io/github/v/tag/ludsoft/Watcharr?label=version&style=for-the-badge" /></a>
  <a href="https://github.com/sbondCo/Watcharr"><img src="https://img.shields.io/badge/upstream-sbondCo%2FWatcharr-blue?style=for-the-badge" /></a>
  <a href="/LICENSE"><img src="https://img.shields.io/github/license/ludsoft/Watcharr?style=for-the-badge" /></a>
</p>

> [!NOTE]
> This is a fork of **[Watcharr](https://github.com/sbondCo/Watcharr)**, made by [sbondCo](https://github.com/sbondCo) and its [contributors](https://github.com/sbondCo/Watcharr/graphs/contributors). All the credit for the app goes to them.
>
> - Upstream project: <https://github.com/sbondCo/Watcharr>
> - Documentation: <https://watcharr.app>
> - Upstream issues: <https://github.com/sbondCo/Watcharr/issues>
> - Upstream releases: <https://github.com/sbondCo/Watcharr/releases>
> - Matrix community: <https://matrix.to/#/#watcharr:matrix.org>
>
> This fork is a set of patches on top of upstream `dev` and follows its updates. Its versions are `<upstream version>-ludsoft.<fork version>` (e.g. `4.4.0-ludsoft.1.2.0`), see the [changelog](CHANGELOG.md). Please report issues with fork features [here](https://github.com/ludsoft/Watcharr/issues), not upstream.

## Features added by this fork

- **OpenID Connect (OIDC) login**: sign in with your identity provider (Keycloak, Authentik, ...). See [OIDC auth](doc/docs/server_config/oidc-auth.md).
- **Personal analytics page** (from the user menu): films, shows, episodes and games watched over a period (this month, this year, last year, last 12 months, all time), with runtimes, a daily activity heatmap and monthly charts. Click a day in the heatmap to see what you watched that day.
- **"Finished at release date"**: next to Finished, records the watch at the release date instead of today. Finishing a whole show finishes each released episode at its own air date.
- **Discover filters**: year range filter for popular movies and shows, and a country filter for popular movies (theatrical releases in that country). Filters are kept in the url.
- **Netflix import**: import your `NetflixViewingHistory.csv` from the Import page, with each play and watched episode on its date. See [Netflix import](doc/docs/importing/netflix.md).

## About Watcharr

I'm your new easily self-hosted content watched list. The place you store your watched (or watching, planned, etc) **movies** and **tv shows** (and **anime**), rate them and track their status.

With [some extra configuration](https://watcharr.app/docs/server_config/game-support-igdb) I can also track your **video games**.

I am built with Go and Svelte(Kit).

See the [list of all current features](FEATURES.md).

### Contents

- [Screenshots](#screenshots)
- [Set Up](#set-up)
- [Community Made Tools](#community-made-tools)
- [Getting Help](#getting-help)
- [License](#license)
- [Contributing](#contributing)

# Screenshots

<p align="center">

<img src="./screenshot/devices-mock.png" alt="Overview" />

| Homepage                                                       | Watched Show Hover                                                          |
| -------------------------------------------------------------- | --------------------------------------------------------------------------- |
| <img src="./screenshot/homepage.png?v=2" alt="Watched List" /> | <img src="./screenshot/homepage-poster-hover.png?v=2" alt="Watched List" /> |

| Watched Show Status Change                                                                  | Movie Details                                                                      |
| ------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| <img src="./screenshot/homepage-poster-change-status.png?v=2" alt="Changing Show Status" /> | <img src="./screenshot/content-details-page.png?v=2" alt="Content Details Page" /> |

| User Profile                                                            | Discover                                                             |
| ----------------------------------------------------------------------- | -------------------------------------------------------------------- |
| <img src="./screenshot/user-profile.png?v=2" alt="User Profile Page" /> | <img src="./screenshot/discover-page.png?v=2" alt="Discover Page" /> |

| Dark Homepage                                                              | Dark Content Details                                                                       |
| -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| <img src="./screenshot/homepage-dark.png?v=2" alt="Dark Theme Homepage" /> | <img src="./screenshot/content-details-page-dark.png?v=2" alt="Dark Theme Content Page" /> |

</p>

# Set Up

Watcharr's [documentation](https://watcharr.app/docs/category/installation) has an up to date guide on setup, and applies to this fork. Just use the fork's image instead of upstream's:

```yaml
image: ghcr.io/ludsoft/watcharr:latest # or a version, e.g. v4.4.0-ludsoft.1.2.0
```

If you hate manuals, but love docker, this [docker-compose.yml](./docker-compose.yml) file (already set to the fork image) is your friend. Images are amd64 only.

# Community Made Tools

Third-party tools made by the community for enhancing your Watcharr experience!

- [Kodi Plugin](https://github.com/airdogvan/watcharr_kodi) by [airdogvan](https://github.com/airdogvan) for automatically tracking your watched shows/movies.

Thanks to anyone that has made a script or tool for Watcharr. Feel free to add your own to the list if you have one!

**Note:** I cannot provide any assurances for these tools or stay on top of them (code review, etc), if you have any problems please open an issue in the project for the tool so that they can stay organized.

# Getting Help

For problems with the features added by this fork, [open an issue on the fork](https://github.com/ludsoft/Watcharr/issues/new).

For everything else, Watcharr has an [issue tracker](https://github.com/sbondCo/Watcharr/issues) and a [space on Matrix](https://matrix.to/#/#watcharr:matrix.org), but please check first that the problem also happens with upstream Watcharr.

# License

This project is licensed under the GPLv3 license. You should see the [LICENSE](LICENSE) file located in the root folder of this project for the full license text, if not, see <https://www.gnu.org/licenses/>.

# Contributing

Please continue to our [contributing guide](CONTRIBUTING.md).
