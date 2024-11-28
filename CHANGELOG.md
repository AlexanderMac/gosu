# Changelog

## v0.4.0 (Nov. 28, 2024)
### BREAKING_CHANGES
- The `CheckUpdates`, `DownloadAsset` and `UpdateApp` now return the `State` result.
### Changes
- Use channel to get asset's downloading progress.
- Add possibility to cancel asset's downloading.

## v0.3.0 (Dec. 05, 2023)
- Add option to download the changelog content.
- Use resty instead of net/http.

## v0.2.0 (Sep. 27, 2023)
### BREAKING_CHANGES
- Delete the `logger` parameter from the `gosu.New` function. To set a custom logger use the `SetLogger` function, it accepts a struct satisfying the `Logger` interface.

## v0.1.0 (Sep. 22, 2023)
- First public release.
