# gosu

[![Build Status](https://github.com/AlexanderMac/gosu/actions/workflows/ci.yml/badge.svg)](https://github.com/AlexanderMac/gosu/actions/workflows/ci.yml)
[![MIT license](https://img.shields.io/badge/license-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)
[![GoDoc](https://pkg.go.dev/badge/github.com/alexandermac/gosu)](https://pkg.go.dev/github.com/alexandermac/gosu)

A package for self updating go applications. Gets the latest application release from the project's Github repository, when a new version is detected, downloads the update archive, upgrades the application and restarts it automatically.
Works in Windows and Linux.

### Install
```sh
go get github.com/alexandermac/gosu
```

### Usage
```go
package main

import (
	"fmt"
	"log"

	"github.com/alexandermac/gosu"
	"github.com/sirupsen/logrus"
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
			logrus.StandardLogger(), // logger
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

### License
Licensed under the MIT license.

### Author
Alexander Mac
