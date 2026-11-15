# Changelog

All notable changes of `multi-tour-worker-payment-expiry` (Payment expiry worker) are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [3.0.0] - 2026-11-14

Cut 3 release. Every entry below is a pull request merged into `qa` and re-applied to `release/3.0.0` with `git cherry-pick -x`; the heading of each group is the user story named in the pull request, and the entry is the title of the pull request without its type. The frozen head of `qa` is `05ee804d4ca072200f4af82517aad6303674993c`.

### Changed

#### HU-WRK-001 - Expire unpaid reservations and balances

- The module and configuration of job 1 (part 1) ([PR 5](https://github.com/code-corhuila/multi-tour-worker-payment-expiry/pull/5))
- The job and its adapters of job 1 (part 2) ([PR 6](https://github.com/code-corhuila/multi-tour-worker-payment-expiry/pull/6))
- The HU-WRK-001 tests ([PR 9](https://github.com/code-corhuila/multi-tour-worker-payment-expiry/pull/9))
- The checks on pull requests to main ([PR 10](https://github.com/code-corhuila/multi-tour-worker-payment-expiry/pull/10))
- Run the test workflow only on pull requests to develop and qa ([PR 12](https://github.com/code-corhuila/multi-tour-worker-payment-expiry/pull/12))

[Unreleased]: https://github.com/code-corhuila/multi-tour-worker-payment-expiry/compare/v3.0.0...HEAD
[3.0.0]: https://github.com/code-corhuila/multi-tour-worker-payment-expiry/releases/tag/v3.0.0
