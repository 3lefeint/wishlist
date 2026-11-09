# Wishlist

A self-hosted gift wishlist app for families and friends.

Users form circles (a family, a group of friends), add people who have no account (children, acquaintances) and collect
gift ideas for each person. Another circle member can claim a gift so nobody buys it twice. The person a list is for does
not see who claimed what.

## Features (planned)

- Circles: a user can belong to several circles and only ever sees their own.
- People without an account, for example children, with their own wishlist.
- Gifts with title, comment, product link, image with a small preview and the person they are for.
- Tags with automatically assigned colors and a matching light or dark text color.
- Claim a gift, and deactivate or delete gifts you created.
- Self-registration with administrator approval, and an admin UI for users, circles, memberships and gifts.

## Stack

Go backend, PostgreSQL, Svelte PWA. Deployed to Kubernetes with Kustomize overlays for staging and production.

## Status

Early development, nothing is released yet.

## License

[MIT](LICENSE)
