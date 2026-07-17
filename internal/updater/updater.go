package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const DefaultRepository = "IDKDeadXD/AutoDeploy"

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

type Result struct {
	Updated bool
	Version string
}

func Update(repository, currentVersion, destination string) (Result, error) {
	if repository == "" {
		repository = DefaultRepository
	}
	if !validRepository(repository) {
		return Result{}, errors.New("release repository must be owner/name")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	release, err := latest(client, repository)
	if err != nil {
		return Result{}, err
	}
	version := strings.TrimPrefix(release.TagName, "v")
	if version == "" {
		return Result{}, errors.New("latest release has no version tag")
	}
	if version == strings.TrimPrefix(currentVersion, "v") {
		return Result{Version: version}, nil
	}
	assetName, err := binaryAssetName()
	if err != nil {
		return Result{}, err
	}
	binary, ok := asset(release.Assets, assetName)
	if !ok {
		return Result{}, fmt.Errorf("release %s has no %s asset", release.TagName, assetName)
	}
	checksums, ok := asset(release.Assets, "checksums.txt")
	if !ok {
		return Result{}, errors.New("release does not include checksums.txt")
	}
	expected, err := checksum(client, checksums.DownloadURL, assetName)
	if err != nil {
		return Result{}, err
	}
	temporary, err := download(client, binary.DownloadURL, filepath.Dir(destination))
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(temporary)
	actual, err := fileChecksum(temporary)
	if err != nil {
		return Result{}, err
	}
	if !strings.EqualFold(expected, actual) {
		return Result{}, errors.New("download checksum does not match release checksum")
	}
	if err := os.Chmod(temporary, 0755); err != nil {
		return Result{}, err
	}
	if err := os.Rename(temporary, destination); err != nil {
		return Result{}, fmt.Errorf("replace deploy binary: %w", err)
	}
	return Result{Updated: true, Version: version}, nil
}

func latest(client *http.Client, repository string) (Release, error) {
	request, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+repository+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "deploy-agent")
	response, err := client.Do(request)
	if err != nil {
		return Release{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub release lookup returned %s", response.Status)
	}
	var release Release
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return Release{}, err
	}
	return release, nil
}

func download(client *http.Client, url, directory string) (string, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "deploy-agent")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("release download returned %s", response.Status)
	}
	file, err := os.CreateTemp(directory, ".deploy-update-")
	if err != nil {
		return "", err
	}
	name := file.Name()
	if _, err := io.Copy(file, io.LimitReader(response.Body, 128<<20)); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return name, nil
}

func checksum(client *http.Client, url, assetName string) (string, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "deploy-agent")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum download returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName && len(fields[0]) == 64 {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %s is missing", assetName)
}

func fileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func asset(assets []Asset, name string) (Asset, bool) {
	for _, candidate := range assets {
		if candidate.Name == name {
			return candidate, true
		}
	}
	return Asset{}, false
}

func binaryAssetName() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "deploy-linux-amd64", nil
	case "arm64":
		return "deploy-linux-arm64", nil
	default:
		return "", fmt.Errorf("unsupported CPU architecture %s", runtime.GOARCH)
	}
}

func validRepository(value string) bool {
	parts := strings.Split(value, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.ContainsAny(value, "\\?&#")
}
