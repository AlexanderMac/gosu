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

	"github.com/alexandermac/gosu"
)

type AppUpdater struct {
	gosu *gosu.Updater
}

func NewUpdater(appVersion string) AppUpdater {
	updater := AppUpdater{
		gosu: gosu.New(
			"alexandermac/superapp", // organization name + project name
			"",                      // github access token to access private repos
			appVersion,              // local version of the app
		),
	}

	return updater
}

func (updater *AppUpdater) CheckUpdates() {
	res, err := updater.gosu.CheckUpdates()
	if err != nil {
		log.Panic(err)
	}

	// gosu.CheckUpdates returns a status code. The code can be used to show information alerts or get the update confirmation from the user
	switch res.Code {
	case gosu.CODE_LATEST_VERSION_IS_USED_ALREADY:
		fmt.Println(res.Message)
	case gosu.CODE_UNRELEASED_VERSION_IS_USED:
		fmt.Println(res.Message)
	case gosu.CODE_UPGRADE_CONFIRMATION:
		fmt.Println(res.Message, res.Details)
	case gosu.CODE_ERROR:
		fmt.Println(res.Message)
	}
}

func (updater *AppUpdater) UpgradeApp() {
	// gosu.UpgradeApp downloads the latest app release from github and upgrades the app
	err := updater.gosu.UpgradeApp()
	if err != nil {
		log.Panic(err)
	}
}

func main() {
	updater := NewUpdater("1.1.0")
	updater.CheckUpdates() // check and print the update status
	updater.UpgradeApp()   // update your local version with the latest version from github
}
```

# API

### New()
Creates a new instance of `gosu.Updater`.

```go
gosu := gosu.New(
	"alexandermac/superapp", // organization name + project name
	"",                      // github access token to access private repos
	appVersion,              // local version of the app
)
```

### SetLogger(l Logger)
Sets a custom logger instead of the standard `log`, used by default. The provided logger must satisfy the `Logger` interface.

```go
gosu.SetLogger(logrus.StandardLogger())
```

# License
Licensed under the MIT license.

# Author
Alexander Mac
