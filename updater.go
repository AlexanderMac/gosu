package gosu

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	semver "github.com/Masterminds/semver/v3"
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
	issuesUrl     string
	localVersion  string
	ghAccessToken string
	logger        _Logger
}

type _CheckUpdatesResult struct {
	Code    string
	Message string
	Details string
}

type _Logger interface {
	Debug(args ...any)
	Info(args ...any)
	Warn(args ...any)
	Error(args ...any)

	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
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

func New(orgRepoName, ghAccessToken, localVersion string, logger _Logger) *Updater {
	return &Updater{
		issuesUrl:     fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", orgRepoName),
		localVersion:  localVersion,
		ghAccessToken: ghAccessToken,
		logger:        logger,
	}
}

func (updater *Updater) CheckUpdates() (_CheckUpdatesResult, error) {
	updater.logger.Info("Checking for updates")

	lastRelease, err := updater.getLastRelease()
	if err != nil {
		return _CheckUpdatesResult{}, err
	}

	remoteSemver := updater.parseSemVer(lastRelease.TagName)
	localSemver := updater.parseSemVer(updater.localVersion)

	if remoteSemver == nil || localSemver == nil {
		return _CheckUpdatesResult{
			Code:    CODE_ERROR,
			Message: "Unable to get updates, the semvver is invalid.",
		}, nil
	}

	// up-to-date
	if remoteSemver.Equal(localSemver) {
		updater.logger.Info("The latest version is already used")
		return _CheckUpdatesResult{
			Code:    CODE_LATEST_VERSION_IS_USED_ALREADY,
			Message: "You already use the latest version.",
		}, nil
	}

	// local version is higher
	if remoteSemver.LessThan(localSemver) {
		updater.logger.Info("The local version is higher than remote")
		return _CheckUpdatesResult{
			Code:    CODE_UNRELEASED_VERSION_IS_USED,
			Message: "You use the unreleased version.",
		}, nil
	}

	// new version detected
	updater.logger.Infof("New version detected %s", lastRelease.TagName)
	return _CheckUpdatesResult{
		Code: CODE_UPGRADE_CONFIRMATION,
		Message: fmt.Sprintf(
			"Upgrade to a new version? Current version is %s, new version is %s.",
			updater.localVersion,
			lastRelease.TagName,
		),
		Details: lastRelease.Body,
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

	updater.logger.Info("Terminating the app")
	os.Exit(0)

	return nil
}

func (updater *Updater) getLastRelease() (_GhRelease, error) {
	updater.logger.Info("Getting the last release")

	res, err := updater.doRequest(http.MethodGet, updater.issuesUrl, nil)
	if err != nil {
		return _GhRelease{}, err
	}
	defer res.Body.Close()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return _GhRelease{}, err
	}
	updater.logger.Debugf("Response Body: %s", resBody)

	var ghRelease _GhRelease
	err = json.Unmarshal(resBody, &ghRelease)
	if err != nil {
		return _GhRelease{}, err
	}

	ghReleaseStr, err := json.MarshalIndent(ghRelease, "", " ")
	if err != nil {
		return _GhRelease{}, err
	}
	updater.logger.Debugf("GhRelease: %v", string(ghReleaseStr))

	updater.logger.Infof("Got the last release: %v successfully", ghRelease)
	return ghRelease, nil
}

func (updater *Updater) downloadAsset(asset _GhReleaseAsset) error {
	updater.logger.Infof("Downloading the asset %s from %s", asset.Name, asset.Url)

	headers := map[string]string{
		"Accept": "application/octet-stream",
	}
	res, err := updater.doRequest(http.MethodGet, asset.Url, headers)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	out, err := os.Create(asset.Name)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, res.Body)
	if err != nil {
		return err
	}

	updater.logger.Infof("Downloaded the asset %s successfully", asset.Name)
	return nil
}

func (updater *Updater) createAndRunExtractor(asset _GhReleaseAsset) error {
	scriptName := fmt.Sprintf(".%s%s", string(os.PathSeparator), asset.updateScriptName)
	updater.logger.Infof("Creating an extractor script %s", scriptName)

	err := os.WriteFile(scriptName, []byte(asset.updateScriptBody), 0777)
	if err != nil {
		return err
	}

	updater.logger.Infof("Running the extractor script %s", scriptName)
	cmd := exec.Command(scriptName)
	err = cmd.Start()
	if err != nil {
		return err
	}
	// don't call cmd.Wait because we need to close the app immediately

	updater.logger.Infof("The extractor script run successfully")
	return nil
}

func (updater *Updater) doRequest(method string, url string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if updater.ghAccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", updater.ghAccessToken))
	}
	//nolint:golint,gosimple
	if headers != nil {
		for headerName, headerValue := range headers {
			req.Header.Set(headerName, headerValue)
		}
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	updater.logger.Debugf("Response: status=%d", res.StatusCode)
	return res, nil
}

func (updater *Updater) parseSemVer(version string) *semver.Version {
	ret, err := semver.NewVersion(version)
	if err != nil {
		if strings.Contains(err.Error(), "Error parsing version segment") {
			updater.logger.Warnf("Unable to parse version, version=%s", version)
		} else {
			updater.logger.Error(err)
		}
		return nil
	}

	return ret
}
