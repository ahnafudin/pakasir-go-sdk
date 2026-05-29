# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-05-30

First stable public release. The public API is now covered by semantic
versioning — breaking changes will only ship in a future major version.
Builds on 0.1.0-alpha.1 (full initial feature set listed below) with:

### Fixed
- `GetPaymentURL` now emits `qris_only=1` (the value Pakasir's hosted page
  expects per the official docs) instead of `qris_only=true`.

### Added
- `examples/sandbox-smoketest`: a maintainer tool that validates the SDK's
  wire format against a live Pakasir sandbox project (create → detail →
  simulate → detail), reading credentials from the environment.

### Validated
- Wire format verified end-to-end against the live Pakasir sandbox API:
  all 18 lifecycle checks passed (GetPaymentURL, CreatePayment, DetailPayment,
  SimulatePayment, and post-payment completion with `completed_at` parsing).
  The `{"payment": …}` and `{"transaction": …}` envelopes, all field
  mappings, and RFC3339 timestamp parsing match the real API.

## [0.1.0-alpha.1] - 2026-05-22

### Added
- Initial public implementation of the Pakasir Go SDK.
- `Client` with functional options (`WithHTTPClient`, `WithBaseURL`,
  `WithTimeout`, `WithUserAgent`, `WithLogger`).
- Payment operations: `CreatePayment`, `GetPaymentURL`, `DetailPayment`,
  `CancelPayment`, `SimulatePayment`.
- `Watch(ctx)` real-time polling helper with status dedup, transient/permanent
  error classification, and ctx-based cancellation.
- Webhook helpers: `ParseWebhook`, `ParseWebhookRequest`, `Event.Match`,
  `Event.Verify`.
- Typed enums: `Method` (10 payment methods) with `IsValid` / `AllMethods`;
  `Status` (4 statuses) with `IsTerminal` / `IsKnown`.
- Typed errors: `*APIError` plus wrappable sentinels (`ErrInvalidOrderID`,
  `ErrInvalidAmount`, `ErrInvalidMethod`, `ErrEmptyBody`, `ErrBodyTooLarge`,
  `ErrAmountMismatch`, `ErrOrderIDMismatch`).
- Fuzz test for `ParseWebhook` (`FuzzParseWebhook`).
- CI matrix on ubuntu/macos/windows running Go 1.26.3.

[Unreleased]: https://github.com/ahnafudin/pakasir-go-sdk/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/ahnafudin/pakasir-go-sdk/compare/v0.1.0-alpha.1...v1.0.0
[0.1.0-alpha.1]: https://github.com/ahnafudin/pakasir-go-sdk/releases/tag/v0.1.0-alpha.1
