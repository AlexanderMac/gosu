<div align="center">
  <h1>gosu</h1>
  <p>A package for self updating Go applications</p>
  <p>
    <a href="https://github.com/alexandermac/gosu/actions/workflows/ci.yml?query=branch%3Amaster"><img src="https://github.com/alexandermac/gosu/actions/workflows/ci.yml/badge.svg" alt="Build Status"></a>
    <a href="https://goreportcard.com/report/github.com/alexandermac/gosu"><img src="https://goreportcard.com/badge/github.com/alexandermac/gosu" alt="Go Report Card"></a>
    <a href="https://pkg.go.dev/github.com/alexandermac/gosu"><img src="https://pkg.go.dev/badge/github.com/alexandermac/gosu.svg" alt="Go Docs"></a>
    <a href="LICENSE"><img src="https://img.shields.io/github/license/alexandermac/gosu.svg" alt="License"></a>
    <a href="https://img.shields.io/github/v/tag/alexandermac/gosu"><img src="https://img.shields.io/github/v/tag/alexandermac/gosu" alt="GitHub tag"></a>
  </p>
</div>

A package for self updating Go applications. Gets the latest application release from the project's Github repository (public or private), when a new version is detected, downloads the update archive, upgrades the application and restarts it automatically.
Works in Windows and Linux.

Works in Go v1.18+.

# Contents
- [Contents](#contents)
- [Install](#install)
- [Usage](#usage)
- [API](#api)
- [License](#license)

# Install
```sh
go get github.com/alexandermac/gosu
```

# Usage
```go
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/alexandermac/gosu"
)

type AppUpdater struct {
	gosu *gosu.Updater
}

func NewUpdater(appVersion string) AppUpdater {
	return AppUpdater{
		gosu: gosu.New(
			os.Getenv("GH_ORG_NAME"),     // organization name + project name
			os.Getenv("GH_ACCESS_TOKEN"), // github access token to access private repos
			appVersion,                   // local version of the app
		),
	}
}

func (updater *AppUpdater) CheckUpdates() bool {
	result := updater.gosu.CheckUpdates()

	switch result.Code {
	case gosu.CODE_LATEST_VERSION_IS_ALREADY_IN_USE, gosu.CODE_UNRELEASED_VERSION_IS_IN_USE, gosu.CODE_NEW_VERSION_DETECTED:
		fmt.Println(">>>", result.Message, result.Details)
	case gosu.CODE_ERROR:
		err := fmt.Errorf("%s. %s", result.Message, result.Details)
		log.Panic(err)
	}

	return result.Code == gosu.CODE_NEW_VERSION_DETECTED
}

func (updater *AppUpdater) DownloadUpdate() bool {
	progressCh := make(chan gosu.DownloadingProgress)
	go func() {
		for {
			progress, ok := <-progressCh
			if !ok {
				return
			}
			fmt.Printf("Asset downloading progress: %d/%d\n", progress.CurrentSize, progress.TotalSize)
		}
	}()

	result := updater.gosu.DownloadAsset(progressCh)
	switch result.Code {
	case gosu.CODE_DOWNLOADING_CANCELLED, gosu.CODE_DOWNLOADING_COMPLETED:
		fmt.Println(">>>", result.Message, result.Details)
	case gosu.CODE_ERROR:
		err := fmt.Errorf("%s. %s", result.Message, result.Details)
		log.Panic(err)
	}

	return result.Code == gosu.CODE_DOWNLOADING_COMPLETED
}

func (updater *AppUpdater) CancelDownloading() {
	updater.gosu.CancelAssetDownloading()
}

func (updater *AppUpdater) UpdateApp() {
	result := updater.gosu.UpdateApp()
	switch result.Code {
	case gosu.CODE_ERROR:
		err := fmt.Errorf("%s. %s", result.Message, result.Details)
		log.Panic(err)
	}
}

func main() {
	updater := NewUpdater("1.3.0")
	result := updater.CheckUpdates()
	if !result {
		return
	}

	go func() {
		time.Sleep(time.Second * 5)
		updater.CancelDownloading()
	}()
	result = updater.DownloadUpdate()
	if !result {
		return
	}

	updater.UpdateApp()
}

```

# API

### New(orgRepoName, ghAccessToken, localVersion string) *Updater
Creates a new instance of `gosu.Updater`.

```go
gosu := gosu.New(
	"alexandermac/superapp", // organization name + project name
	"",                      // github access token to access private repos
	"1.3.0",                 // local version of the app
)
```

### SetLogger(l Logger)
Sets a custom logger instead of the standard `log` used by default. The provided logger must satisfy the `Logger` interface.

```go
gosu.SetLogger(logrus.StandardLogger())
```

### CheckUpdates() UpdateResult
Checks for application's updates. Returns struct with code and message indicating that new version exists or not. 

### DownloadAsset(progressCh chan<- DownloadingProgress) UpdateResult
Downloads a release asset. Accepts an optional channel to get downloading progress notifications.

### CancelAssetDownloading()
Cancels an asset downloading.

### UpdateApp() UpdateResult
Installs the downloaded update.

# License
Licensed under the MIT license.

# Author
Alexander Mac
