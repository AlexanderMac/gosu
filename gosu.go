package gosu

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/go-resty/resty/v2"
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
	CODE_LATEST_VERSION_IS_USED_ALREADY = "LATEST_VERSION_IS_USED_ALREADY"
	CODE_UNRELEASED_VERSION_IS_USED     = "UNRELEASED_VERSION_IS_USED"
	CODE_UPGRADE_CONFIRMATION           = "UPGRADE_CONFIRMATION"
	CODE_ERROR                          = "ERROR"
)

type Updater struct {
	ReleasesUrl       string
	ChangelogUrl      string
	LocalVersion      string
	GhAccessToken     string
	DownloadChangelog bool
}

type _CheckUpdatesResult struct {
	Code    string
	Message string
	Details string
}

type _GhReleaseAsset struct {
	Name             string `json:"name"`
	Url              string `json:"url"`
	updateScriptName string
	updateScriptBody string
}

type _GhRelease struct {
	TagName   string            `json:"tag_name"`
	CreatedAt string            `json:"created_at"`
	Assets    []_GhReleaseAsset `json:"assets"`
	Body      string            `json:"body"`
}

func New(orgRepoName, ghAccessToken, localVersion string) *Updater {
	return &Updater{
		ReleasesUrl:   fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", orgRepoName),
		ChangelogUrl:  fmt.Sprintf("https://api.github.com/repos/%s/contents/CHANGELOG.md", orgRepoName),
		LocalVersion:  localVersion,
		GhAccessToken: ghAccessToken,
	}
}

func (updater *Updater) CheckUpdates() (_CheckUpdatesResult, error) {
	logger.Info("Checking for updates")

	lastRelease, err := updater.getLastRelease()
	if err != nil {
		return _CheckUpdatesResult{
			Code:    CODE_ERROR,
			Message: fmt.Sprintf("Unable to get updates. %s.", updater.parseHttpError(err)),
		}, nil
	}

	remoteSemver := updater.parseSemVer(lastRelease.TagName)
	localSemver := updater.parseSemVer(updater.LocalVersion)

	if remoteSemver == nil || localSemver == nil {
		return _CheckUpdatesResult{
			Code:    CODE_ERROR,
			Message: "Unable to get updates. The semvver is invalid.",
		}, nil
	}

	// up-to-date
	if remoteSemver.Equal(localSemver) {
		logger.Info("The latest version is already used")
		return _CheckUpdatesResult{
			Code:    CODE_LATEST_VERSION_IS_USED_ALREADY,
			Message: "You already use the latest version.",
		}, nil
	}

	// local version is higher
	if remoteSemver.LessThan(localSemver) {
		logger.Info("The local version is higher than remote")
		return _CheckUpdatesResult{
			Code:    CODE_UNRELEASED_VERSION_IS_USED,
			Message: "You use the unreleased version.",
		}, nil
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
	return _CheckUpdatesResult{
		Code: CODE_UPGRADE_CONFIRMATION,
		Message: fmt.Sprintf(
			"Upgrade to a new version? Current version is %s, new version is %s.",
			updater.LocalVersion,
			lastRelease.TagName,
		),
		Details: lastReleaseDetails,
	}, nil
}

func (updater *Updater) UpgradeApp() error {
	lastRelease, err := updater.getLastRelease()
	if err != nil {
		return err
	}

	var asset _GhReleaseAsset
	if strings.Contains(runtime.GOOS, "linux") {
		assetIndex := slices.IndexFunc(lastRelease.Assets, func(asset _GhReleaseAsset) bool {
			return strings.Contains(asset.Name, "-linux")
		})
		asset = lastRelease.Assets[assetIndex]
		asset.updateScriptName = _LINUX_SCRIPT_NAME
		asset.updateScriptBody = linuxScript
	} else if strings.Contains(runtime.GOOS, "windows") {
		assetIndex := slices.IndexFunc(lastRelease.Assets, func(asset _GhReleaseAsset) bool {
			return strings.Contains(asset.Name, "-win")
		})
		asset = lastRelease.Assets[assetIndex]
		asset.updateScriptName = _WIN_SCRIPT_NAME
		asset.updateScriptBody = windowsScript
	} else {
		err := errors.New("Unsupported OS: " + runtime.GOOS)
		if err != nil {
			return err
		}
	}

	err = updater.downloadAsset(asset)
	if err != nil {
		return err
	}

	err = updater.createAndRunExtractor(asset)
	if err != nil {
		return err
	}

	logger.Info("Terminating the app")
	os.Exit(0)

	return nil
}

func (updater *Updater) getLastRelease() (_GhRelease, error) {
	logger.Info("Getting the last release")

	var ghRelease _GhRelease
	res, err := updater.getHttpClient().
		R().
		SetResult(&ghRelease).
		Get(updater.ReleasesUrl)
	if err != nil {
		return _GhRelease{}, err
	}
	if err := updater.checkHttpResponse(res); err != nil {
		return _GhRelease{}, err
	}

	logger.Infof("Got the last release: %v successfully", ghRelease)
	return ghRelease, nil
}

func (updater *Updater) getChangelog() (string, error) {
	logger.Info("Getting the changelog")

	res, err := updater.getHttpClient().
		R().
		SetHeader("Accept", "application/vnd.github.raw").
		Get(updater.ChangelogUrl)
	if err != nil {
		return "", err
	}
	if err := updater.checkHttpResponse(res); err != nil {
		return "", err
	}

	logger.Info("Got the changelog successfully")
	return string(res.Body()), nil
}

func (updater *Updater) downloadAsset(asset _GhReleaseAsset) error {
	logger.Infof("Downloading the asset %s from %s", asset.Name, asset.Url)

	res, err := updater.getHttpClient().
		R().
		SetHeader("Accept", "application/octet-stream").
		SetOutput(asset.Name).
		Get(asset.Url)
	if err != nil {
		return err
	}
	if err := updater.checkHttpResponse(res); err != nil {
		return err
	}

	logger.Infof("Downloaded the asset %s successfully", asset.Name)
	return nil
}

func (updater *Updater) createAndRunExtractor(asset _GhReleaseAsset) error {
	scriptName := fmt.Sprintf(".%s%s", string(os.PathSeparator), asset.updateScriptName)
	logger.Infof("Creating an extractor script %s", scriptName)

	err := os.WriteFile(scriptName, []byte(asset.updateScriptBody), 0777)
	if err != nil {
		return err
	}

	logger.Infof("Running the extractor script %s", scriptName)
	cmd := exec.Command(scriptName)
	err = cmd.Start()
	if err != nil {
		return err
	}
	// don't call cmd.Wait because we need to close the app immediately

	logger.Infof("The extractor script run successfully")
	return nil
}

func (updater *Updater) getHttpClient() *resty.Client {
	client := resty.New()
	client.Header.Set("Accept", "application/vnd.github+json")
	client.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if updater.GhAccessToken != "" {
		client.Header.Set("Authorization", fmt.Sprintf("Bearer %s", updater.GhAccessToken))
	}

	return client
}

func (updater *Updater) checkHttpResponse(res *resty.Response) error {
	if res.IsError() {
		return fmt.Errorf("Request failed with status: %d. %s", res.StatusCode(), res.String())
	}
	return nil
}

func (updater *Updater) parseHttpError(err error) string {
	if strings.Contains(err.Error(), "connection refused") {
		return "Error on connect to remote server"
	}
	return err.Error()
}

func (updater *Updater) parseSemVer(version string) *semver.Version {
	ret, err := semver.NewVersion(version)
	if err != nil {
		if strings.Contains(err.Error(), "Error parsing version segment") {
			logger.Warnf("Unable to parse version, version=%s", version)
		} else {
			logger.Error(err)
		}
		return nil
	}

	return ret
}
