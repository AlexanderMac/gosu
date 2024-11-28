package gosu

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/exp/slices"
)

//go:embed scripts/linux.txt
var linuxScript string

//go:embed scripts/windows.txt
var windowsScript string

const (
	_LINUX_SCRIPT_NAME = "update.sh"
	_WIN_SCRIPT_NAME   = "update.cmd"
)

const (
	// CheckUpdates
	CODE_LATEST_VERSION_IS_ALREADY_IN_USE = iota
	CODE_UNRELEASED_VERSION_IS_IN_USE     = iota
	CODE_NEW_VERSION_DETECTED             = iota
	// DownloadAsset
	CODE_DOWNLOADING_CANCELLED = iota
	CODE_DOWNLOADING_COMPLETED = iota
	CODE_ERROR                 = iota
)

type Updater struct {
	ReleasesUrl       string
	ChangelogUrl      string
	LocalVersion      string
	GhAccessToken     string
	DownloadChangelog bool
	lastRelease       *_GhRelease
	releaseAsset      *_GhReleaseAsset
	downloadingCtx    context.Context
	cancelDownloading context.CancelFunc
}

type UpdateResult struct {
	Code    int
	Message string
	Details string
}

type DownloadingProgress struct {
	TotalSize       int
	CurrentSize     int
	ProgressPercent int
}

type _GhRelease struct {
	TagName   string            `json:"tag_name"`
	CreatedAt string            `json:"created_at"`
	Assets    []_GhReleaseAsset `json:"assets"`
	Body      string            `json:"body"`
}

type _GhReleaseAsset struct {
	Name             string `json:"name"`
	Url              string `json:"url"`
	Size             int    `json:"size"`
	updateScriptName string
	updateScriptBody string
}

func New(orgRepoName, ghAccessToken, localVersion string) *Updater {
	return &Updater{
		ReleasesUrl:   fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", orgRepoName),
		ChangelogUrl:  fmt.Sprintf("https://api.github.com/repos/%s/contents/CHANGELOG.md", orgRepoName),
		LocalVersion:  localVersion,
		GhAccessToken: ghAccessToken,
	}
}

func (updater *Updater) CheckUpdates() UpdateResult {
	logger.Info("Checking for updates")

	lastRelease, err := updater.getLastRelease()
	if err != nil {
		return UpdateResult{
			Code:    CODE_ERROR,
			Message: "Unable to get updates.",
			Details: parseHttpError(err),
		}
	}
	updater.lastRelease = &lastRelease

	remoteSemver := parseSemVer(lastRelease.TagName)
	localSemver := parseSemVer(updater.LocalVersion)

	if remoteSemver == nil || localSemver == nil {
		return UpdateResult{
			Code:    CODE_ERROR,
			Message: "Unable to get updates. The SemVer is invalid.",
		}
	}

	// up-to-date
	if remoteSemver.Equal(localSemver) {
		logger.Info("The latest version is already used")
		return UpdateResult{
			Code:    CODE_LATEST_VERSION_IS_ALREADY_IN_USE,
			Message: "You already use the latest version.",
		}
	}

	// local version is higher
	if remoteSemver.LessThan(localSemver) {
		logger.Info("The local version is higher than remote")
		return UpdateResult{
			Code:    CODE_UNRELEASED_VERSION_IS_IN_USE,
			Message: "You use the unreleased version.",
		}
	}

	// new version detected
	lastReleaseDetails := lastRelease.Body
	if updater.DownloadChangelog {
		changelog, err := updater.getChangelog()
		if err != nil {
			logger.Warnf("Unable to download changelog, error: %s", err.Error())
		}
		lastReleaseDetails = changelog
	}

	logger.Infof("New version detected %s", lastRelease.TagName)
	return UpdateResult{
		Code: CODE_NEW_VERSION_DETECTED,
		Message: fmt.Sprintf(
			"New version detected. Current version is %s, new version is %s. Download update?",
			updater.LocalVersion,
			lastRelease.TagName,
		),
		Details: lastReleaseDetails,
	}
}

func (updater *Updater) DownloadAsset(progressCh chan<- DownloadingProgress) UpdateResult {
	if updater.lastRelease == nil {
		panic(errors.New("lastRelease is nil"))
	}

	stopCh := make(chan bool)
	defer func() {
		if progressCh != nil {
			close(progressCh)
		}
		close(stopCh)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	updater.downloadingCtx = ctx
	updater.cancelDownloading = cancel

	var asset _GhReleaseAsset
	if strings.Contains(runtime.GOOS, "linux") {
		assetIndex := slices.IndexFunc(updater.lastRelease.Assets, func(a _GhReleaseAsset) bool {
			return strings.Contains(a.Name, "-linux")
		})
		asset = updater.lastRelease.Assets[assetIndex]
		asset.updateScriptName = _LINUX_SCRIPT_NAME
		asset.updateScriptBody = linuxScript
	} else if strings.Contains(runtime.GOOS, "windows") {
		assetIndex := slices.IndexFunc(updater.lastRelease.Assets, func(a _GhReleaseAsset) bool {
			return strings.Contains(a.Name, "-win")
		})
		asset = updater.lastRelease.Assets[assetIndex]
		asset.updateScriptName = _WIN_SCRIPT_NAME
		asset.updateScriptBody = windowsScript
	} else {
		return UpdateResult{
			Code:    CODE_ERROR,
			Message: "Unsupported OS: " + runtime.GOOS,
		}
	}
	updater.releaseAsset = &asset

	err := removeFile(asset.Name)
	if err != nil {
		return UpdateResult{
			Code:    CODE_ERROR,
			Message: "Unable to delete the old asset: " + asset.Name,
			Details: err.Error(),
		}
	}

	if progressCh != nil {
		go getDownloadingPercent(progressCh, stopCh, asset.Name, asset.Size)
	}

	err = updater.downloadAsset(&asset)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return UpdateResult{
				Code:    CODE_DOWNLOADING_CANCELLED,
				Message: "The asset downloading has been cancelled.",
			}
		}
		return UpdateResult{
			Code:    CODE_ERROR,
			Message: "Unable to download asset.",
			Details: err.Error(),
		}
	}

	if progressCh != nil {
		progressCh <- DownloadingProgress{
			TotalSize:       asset.Size,
			CurrentSize:     asset.Size,
			ProgressPercent: 100,
		}
	}

	return UpdateResult{
		Code:    CODE_DOWNLOADING_COMPLETED,
		Message: "A new application version downloaded successfully. Restart to complete update?",
	}
}

func (updater *Updater) CancelAssetDownloading() {
	if updater.cancelDownloading == nil {
		panic(errors.New("cancelDownloading is nil"))
	}

	logger.Info("Cancel the asset downloading")
	updater.cancelDownloading()
}

func (updater *Updater) UpdateApp() UpdateResult {
	if updater.releaseAsset == nil {
		panic(errors.New("releaseAsset is nil"))
	}

	err := createAndRunExtractor(updater.releaseAsset)
	if err != nil {
		return UpdateResult{
			Code:    CODE_ERROR,
			Message: "Unable to create or run extractor.",
			Details: err.Error(),
		}
	}

	logger.Info("Terminating the app")
	os.Exit(0)

	return UpdateResult{}
}

func (updater *Updater) getLastRelease() (_GhRelease, error) {
	logger.Debug("Getting the last release")

	var ghRelease _GhRelease
	res, err := getHttpClient(updater.GhAccessToken).
		R().
		SetResult(&ghRelease).
		Get(updater.ReleasesUrl)
	if err != nil {
		return _GhRelease{}, err
	}
	if err := checkHttpResponse(res); err != nil {
		return _GhRelease{}, err
	}

	logger.Debugf("The last release: %v has been gotten successfully", ghRelease)
	return ghRelease, nil
}

func (updater *Updater) getChangelog() (string, error) {
	logger.Debug("Getting the changelog")

	res, err := getHttpClient(updater.GhAccessToken).
		R().
		SetHeader("Accept", "application/vnd.github.raw").
		Get(updater.ChangelogUrl)
	if err != nil {
		return "", err
	}
	if err := checkHttpResponse(res); err != nil {
		return "", err
	}

	logger.Debug("The changelog has been gotten successfully")
	return string(res.Body()), nil
}

func (updater *Updater) downloadAsset(asset *_GhReleaseAsset) error {
	logger.Debugf("Downloading the asset %s from %s", asset.Name, asset.Url)

	res, err := getHttpClient(updater.GhAccessToken).
		SetRetryCount(3).
		SetTimeout(time.Minute).
		R().
		SetContext(updater.downloadingCtx).
		SetHeader("Accept", "application/octet-stream").
		SetOutput(asset.Name).
		Get(asset.Url)
	if err != nil {
		return err
	}
	if err := checkHttpResponse(res); err != nil {
		return err
	}

	logger.Debugf("The asset %s has been downloaded successfully", asset.Name)
	return nil
}

func getDownloadingPercent(progressCh chan<- DownloadingProgress, stopCh <-chan bool, fileName string, totalSize int) {
	for {
		select {
		case <-stopCh:
			return
		default:
			file, err := os.Open(fileName)
			if err != nil {
				if os.IsNotExist(err) {
					break
				}
				logger.Error(err)
				return
			}
			fi, err := file.Stat()
			if err != nil {
				logger.Error(err)
				return
			}
			currentSize := fi.Size()
			if currentSize == 0 {
				currentSize = 1
			}

			progressPercent := float64(currentSize) / float64(totalSize) * 100
			progressCh <- DownloadingProgress{
				TotalSize:       totalSize,
				CurrentSize:     int(currentSize),
				ProgressPercent: int(progressPercent),
			}
		}
		time.Sleep(time.Millisecond * 25)
	}
}
