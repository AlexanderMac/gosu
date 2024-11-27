package gosu

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
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
	CODE_LATEST_VERSION_IS_ALREADY_IN_USE = "LATEST_VERSION_IS_ALREADY_IN_USE"
	CODE_UNRELEASED_VERSION_IS_IN_USE     = "UNRELEASED_VERSION_IS_IN_USE"
	CODE_NEW_VERSION_DETECTED             = "NEW_VERSION_DETECTED"
	CODE_DOWNLOADING_STARTED              = "DOWNLOADING_STARTED"
	CODE_DOWNLOADING_COMPLETED            = "DOWNLOADING_COMPLETED"
	CODE_ERROR                            = "ERROR"
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

type State struct {
	Code    string
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

func (updater *Updater) CheckUpdates() State {
	logger.Info("Checking for updates")

	lastRelease, err := updater.getLastRelease()
	if err != nil {
		return State{
			Code:    CODE_ERROR,
			Message: "Unable to get updates.",
			Details: parseHttpError(err),
		}
	}
	updater.lastRelease = &lastRelease

	remoteSemver := parseSemVer(lastRelease.TagName)
	localSemver := parseSemVer(updater.LocalVersion)

	if remoteSemver == nil || localSemver == nil {
		return State{
			Code:    CODE_ERROR,
			Message: "Unable to get updates. The semvver is invalid.",
		}
	}

	// up-to-date
	if remoteSemver.Equal(localSemver) {
		logger.Info("The latest version is already used")
		return State{
			Code:    CODE_LATEST_VERSION_IS_ALREADY_IN_USE,
			Message: "You already use the latest version.",
		}
	}

	// local version is higher
	if remoteSemver.LessThan(localSemver) {
		logger.Info("The local version is higher than remote")
		return State{
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
		if changelog != "" {
			lastReleaseDetails = changelog
		}
	}

	logger.Infof("New version detected %s", lastRelease.TagName)
	return State{
		Code: CODE_NEW_VERSION_DETECTED,
		Message: fmt.Sprintf(
			"New version detected. Current version is %s, new version is %s. Download update?",
			updater.LocalVersion,
			lastRelease.TagName,
		),
		Details: lastReleaseDetails,
	}
}

func (updater *Updater) DownloadAsset(stateCh chan<- State, progressCh chan<- DownloadingProgress) {
	if updater.lastRelease == nil {
		panic(errors.New("lastRelease is nil"))
	}

	doneDownloadingCh := make(chan bool)
	defer func() {
		close(progressCh)
		close(doneDownloadingCh)
		close(stateCh)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	updater.downloadingCtx = ctx
	updater.cancelDownloading = cancel

	var asset _GhReleaseAsset
	if strings.Contains(runtime.GOOS, "linux") {
		assetIndex := slices.IndexFunc(updater.lastRelease.Assets, func(asset _GhReleaseAsset) bool {
			return strings.Contains(asset.Name, "-linux")
		})
		asset = updater.lastRelease.Assets[assetIndex]
		asset.updateScriptName = _LINUX_SCRIPT_NAME
		asset.updateScriptBody = linuxScript
	} else if strings.Contains(runtime.GOOS, "windows") {
		assetIndex := slices.IndexFunc(updater.lastRelease.Assets, func(asset _GhReleaseAsset) bool {
			return strings.Contains(asset.Name, "-win")
		})
		asset = updater.lastRelease.Assets[assetIndex]
		asset.updateScriptName = _WIN_SCRIPT_NAME
		asset.updateScriptBody = windowsScript
	} else {
		stateCh <- State{
			Code:    CODE_ERROR,
			Message: "Unsupported OS: " + runtime.GOOS,
		}
		return
	}
	updater.releaseAsset = &asset

	err := removeFile(asset.Name)
	if err != nil {
		stateCh <- State{
			Code:    CODE_ERROR,
			Message: "Unable to delete the previous asset: " + asset.Name,
			Details: err.Error(),
		}
		return
	}

	stateCh <- State{
		Code: CODE_DOWNLOADING_STARTED,
	}

	assetSize, err := updater.getAssetSize(&asset)
	if err != nil {
		stateCh <- State{
			Code:    CODE_ERROR,
			Message: "Unable to download asset.",
			Details: err.Error(),
		}
		return
	}

	go getDownloadingPercent(progressCh, doneDownloadingCh, asset.Name, assetSize)

	err = updater.downloadAsset(&asset)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		stateCh <- State{
			Code:    CODE_ERROR,
			Message: "Unable to download asset.",
			Details: err.Error(),
		}
		return
	}

	progressCh <- DownloadingProgress{
		TotalSize:       assetSize,
		CurrentSize:     assetSize,
		ProgressPercent: 100,
	}
	stateCh <- State{
		Code:    CODE_DOWNLOADING_COMPLETED,
		Message: "A new application version downloaded successfully. Restart to complete update?",
	}
}

func (updater *Updater) CancelDownloadingAsset() error {
	if updater.cancelDownloading == nil {
		return errors.New("cancelDownloading is nil")
	}

	logger.Info("Cancel downloading")
	updater.cancelDownloading()

	return nil
}

func (updater *Updater) UpdateApp() (State, error) {
	if updater.releaseAsset == nil {
		return State{}, errors.New("releaseAsset is nil")
	}

	err := createAndRunExtractor(updater.releaseAsset)
	if err != nil {
		return State{
			Code:    CODE_ERROR,
			Message: "Unable to create and run extractor.",
			Details: err.Error(),
		}, nil
	}

	logger.Info("Terminating the app")
	os.Exit(0)

	return State{}, nil
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

	logger.Debugf("Got the last release: %v successfully", ghRelease)
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

	logger.Debug("Got the changelog successfully")
	return string(res.Body()), nil
}

func (updater *Updater) getAssetSize(asset *_GhReleaseAsset) (int, error) {
	logger.Debugf("Getting the asset size %s from %s", asset.Name, asset.Url)

	res, err := getHttpClient(updater.GhAccessToken).
		R().
		SetHeader("Accept", "application/octet-stream").
		Head(asset.Url)
	if err != nil {
		return 0, err
	}
	if err := checkHttpResponse(res); err != nil {
		return 0, err
	}

	assetSize, err := strconv.Atoi(res.Header().Get("Content-Length"))
	if err != nil {
		return 0, err
	}

	logger.Debugf("Got the asset size %s successfully", asset.Name)
	return assetSize, nil
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

	logger.Debugf("Downloaded the asset %s successfully", asset.Name)
	return nil
}

func getDownloadingPercent(progressCh chan<- DownloadingProgress, doneCh <-chan bool, fileName string, totalSize int) {
	for {
		select {
		case <-doneCh:
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
		time.Sleep(time.Millisecond * 50)
	}
}
