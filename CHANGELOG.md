## v0.3.0
#### _Dec. 5, 2023_
* Add option to download the changelog content.
* Use resty instead of net/http.

## v0.2.0
#### _Sep. 27, 2023_
 * BREAKING_CHANGE: Delete the `logger` parameter from the `gosu.New` function. To set a custom logger now the new `SetLogger` function should be used, it accepts a struct satisfying the `Logger` interface.

## v0.1.0
#### _Sep. 22, 2023_
 * First public release.
