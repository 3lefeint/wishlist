# Changelog

## [Unreleased]

### Added

- Project README and MIT license
- Backend skeleton with health check and Cloudflare Access JWT verification, Dockerfile and Kustomize overlays for staging and production
- PostgreSQL connection, migrations and registration of users as pending on first sign-in
- Admin API to list, approve, block and delete users
- Frontend shell (Svelte, German and English) with sign-in states, installable as an app, served by the backend
- Circles with invite links, members and persons without an account, with display names and guardians
- Gifts with tags, one downscaled image, claims and deactivation
- Admin page to approve, block and delete users, with a badge for waiting registrations
- Profile with first name, last name, birthday and favourite colour, asked after the first sign-in and shown to circle members; optional birthday for persons without an account (#11)
- Admin pages to manage circles, memberships and gifts (#12)

### Changed

- The operator account manages circles, persons, gifts and tags without being a circle member; it cannot create or join circles, claim gifts or appear in member lists (#5)
