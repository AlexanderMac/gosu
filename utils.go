package gosu

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/go-resty/resty/v2"
)

func removeFile(fileName string) error {
	err := os.Remove(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return nil
}

func createAndRunExtractor(asset *_GhReleaseAsset) error {
	scriptName := fmt.Sprintf(".%s%s", string(os.PathSeparator), asset.updateScriptName)
	logger.Debugf("Creating an extractor script %s", scriptName)

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

	logger.Debugf("The extractor script run successfully")
	return nil
}

func getHttpClient(ghAccessToken string) *resty.Client {
	client := resty.New()
	client.Header.Set("Accept", "application/vnd.github+json")
	client.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if ghAccessToken != "" {
		client.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ghAccessToken))
	}

	return client
}

func checkHttpResponse(res *resty.Response) error {
	if res.IsError() {
		return fmt.Errorf("Request failed with status code: %d. %s", res.StatusCode(), res.String())
	}
	return nil
}

func parseHttpError(err error) string {
	if strings.Contains(err.Error(), "connection refused") {
		return "Unable to connect to the server"
	}
	return err.Error()
}

func parseSemVer(version string) *semver.Version {
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
