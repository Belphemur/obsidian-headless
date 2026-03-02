# Changelog

## 0.0.4

- Fixed version command always gave "1.0.0".
- Fixed config syncing deletes remote config files after downloading them.

## 0.0.3

- Removed keytar in favor of flat file configuration, as it is not available in many headless setups.
- Fix download freeze caused by a race condition in the data receiving pipeline, due to microtask queuing differences between node and web (credits to @marcus-crane).

## 0.0.2

- Updated README.md

## 0.0.1

- Initial release.
