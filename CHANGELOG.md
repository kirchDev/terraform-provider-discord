# Changelog

## [0.7.0](https://github.com/kirchDev/terraform-provider-discord/compare/v0.6.0...v0.7.0) (2026-08-31)


### ⚠ BREAKING CHANGES

* **onboarding:** key prompts and options on a caller-supplied key ([#37](https://github.com/kirchDev/terraform-provider-discord/issues/37))

### Features

* **queue:** cut the queue branch on the first worker PR ([#33](https://github.com/kirchDev/terraform-provider-discord/issues/33)) ([b88e630](https://github.com/kirchDev/terraform-provider-discord/commit/b88e630a41f46793d3be81e9279e4152ca79f74a))


### Bug Fixes

* **ci:** let the queue PR body wrap itself ([374708b](https://github.com/kirchDev/terraform-provider-discord/commit/374708b2ff0d5e5aded114a1ea31c4176188321d))
* **ci:** read the Queue App PEM from this owner's own -ci mirror ([127776e](https://github.com/kirchDev/terraform-provider-discord/commit/127776e5cd0b83962db064b916f4a086f4b67dc6))
* **deps:** update the dependencies across Go, npm and Actions ([#40](https://github.com/kirchDev/terraform-provider-discord/issues/40)) ([7919ef1](https://github.com/kirchDev/terraform-provider-discord/commit/7919ef1dd9e735b75c169e42d8a3a00b1dd9b88d))
* **onboarding:** key prompts and options on a caller-supplied key ([#37](https://github.com/kirchDev/terraform-provider-discord/issues/37)) ([40d5ff1](https://github.com/kirchDev/terraform-provider-discord/commit/40d5ff10b9c9e2ee11a7463df979816d19f115db))
* **order:** order listed roles and channels without displacing their siblings ([#38](https://github.com/kirchDev/terraform-provider-discord/issues/38)) ([5f541fb](https://github.com/kirchDev/terraform-provider-discord/commit/5f541fbadd500944e05d6ae1727542acae375f59))

## [0.6.0](https://github.com/kirchDev/terraform-provider-discord/compare/v0.5.0...v0.6.0) (2026-07-26)


### Features

* route questions, ideas and possible bugs to the Discord forum ([6c2a2cc](https://github.com/kirchDev/terraform-provider-discord/commit/6c2a2cc7311dfd5f90ebd8115d12ad5b9e7e9fec))


### Bug Fixes

* align issue-template labels with the label catalog ([9125b0d](https://github.com/kirchDev/terraform-provider-discord/commit/9125b0d08b7e10cc959a083fd5f9471086245405))

## [0.5.0](https://github.com/kirchDev/terraform-provider-discord/compare/v0.4.0...v0.5.0) (2026-06-26)


### Features

* add discord_message resource ([de68540](https://github.com/kirchDev/terraform-provider-discord/commit/de68540be4245acf8cc73f4c6443ac2ce2623719))

## [0.4.0](https://github.com/kirchDev/terraform-provider-discord/compare/v0.3.2...v0.4.0) (2026-06-26)


### Features

* add discord_member_verification resource ([5accd40](https://github.com/kirchDev/terraform-provider-discord/commit/5accd403c926665863e0868557c91636abad30b3))

## [0.3.2](https://github.com/kirchDev/terraform-provider-discord/compare/v0.3.1...v0.3.2) (2026-06-25)


### Bug Fixes

* **onboarding:** stop inconsistent result on nested ids and descriptions ([8302ed2](https://github.com/kirchDev/terraform-provider-discord/commit/8302ed2d0018714a58e47f992485834c8d233f37))

## [0.3.1](https://github.com/kirchDev/terraform-provider-discord/compare/v0.3.0...v0.3.1) (2026-06-25)


### Bug Fixes

* align dependabot labels to the stack: convention ([540c269](https://github.com/kirchDev/terraform-provider-discord/commit/540c269c3ea873a6ef7887ecb5134fed90512e4c))
* **channels:** never send invalid enum 0 on update ([40bdada](https://github.com/kirchDev/terraform-provider-discord/commit/40bdada4daed4f643fe063a5b04cf58d8de9144b))

## [0.3.0](https://github.com/kirchDev/terraform-provider-discord/compare/v0.2.0...v0.3.0) (2026-06-25)


### ⚠ BREAKING CHANGES

* **onboarding:** the prompts_json attribute is removed; model onboarding prompts with the structured prompts block instead.

### Features

* **onboarding:** model prompts as structured attributes ([8a28067](https://github.com/kirchDev/terraform-provider-discord/commit/8a280679cfce02006ea59a8499b5645db9f10843))

## [0.2.0](https://github.com/kirchDev/terraform-provider-discord/compare/v0.1.0...v0.2.0) (2026-06-25)


### Features

* **automod:** support mention_raid_protection_enabled ([c218d84](https://github.com/kirchDev/terraform-provider-discord/commit/c218d84de4fce3cf468c75d8f0237fb895e29af2))

## 0.1.0 (2026-06-24)


### Features

* add discord_channel_order and surface guild features ([84c2ab6](https://github.com/kirchDev/terraform-provider-discord/commit/84c2ab60af2e74a82f452e891266650f1ef0937f))
* add discord_member_role, discord_forum_post and discord_application ([27f7144](https://github.com/kirchDev/terraform-provider-discord/commit/27f71447c72f64323a065cd66d69be4c083297ed))
* add discord_role_order and surface the guild vanity url code ([6578f92](https://github.com/kirchDev/terraform-provider-discord/commit/6578f92d4e6f0e8d60016b095065f2d4409bcf99))
* add stage instance, guild template, read data sources and settable attributes ([d0c94c0](https://github.com/kirchDev/terraform-provider-discord/commit/d0c94c0edef4675eb59edc21d70f81592ef37e13))
* add stickers, soundboard sounds, application emojis and image attributes ([d84cf1d](https://github.com/kirchDev/terraform-provider-discord/commit/d84cf1d5d626f85de605143ff4656e6e9f1096d7))
* discord provider with rest client, resources and data sources ([f486a42](https://github.com/kirchDev/terraform-provider-discord/commit/f486a42faf5f3f9b3434bc09075a23869a8fe610))


### Bug Fixes

* **invite:** keep max_age and max_uses from the create response ([12aec50](https://github.com/kirchDev/terraform-provider-discord/commit/12aec5033f446ed48a819c7ed252b0f109f723cb))
* mark API-defaulted attributes as computed ([0f09674](https://github.com/kirchDev/terraform-provider-discord/commit/0f09674b35ed1933d86e4980ffc565e7741e673b))
* **member_nickname:** set the bot's own nickname via /members/[@me](https://github.com/me) ([998597c](https://github.com/kirchDev/terraform-provider-discord/commit/998597cd1b0d7b67d53c6d12f8b6485f5f3609e1))
* **scheduled_event:** treat equivalent RFC3339 timestamps as unchanged ([342aef2](https://github.com/kirchDev/terraform-provider-discord/commit/342aef261864c8cd168f034cd2322c96d17316cd))

## Changelog

<!-- release-please populates this file from Conventional Commits on the first release. -->
