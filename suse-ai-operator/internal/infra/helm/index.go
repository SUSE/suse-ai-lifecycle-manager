package helm

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

type IndexFile struct {
	Entries map[string][]ChartVersion `yaml:"entries"`
}

type ChartVersion struct {
	Version     string            `yaml:"version"`
	Annotations map[string]string `yaml:"annotations"`
}

func FetchIndex(ctx context.Context, url string) (*IndexFile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch index.yaml: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var index IndexFile
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, err
	}

	return &index, nil
}

func FindAnnotations(
	index *IndexFile,
	chartName string,
	version string,
) (map[string]string, error) {

	versions, ok := index.Entries[chartName]
	if !ok {
		return nil, fmt.Errorf("chart %q not found in index", chartName)
	}

	for _, v := range versions {
		if v.Version == version {
			return v.Annotations, nil
		}
	}

	return nil, fmt.Errorf(
		"version %q not found for chart %q",
		version,
		chartName,
	)
}

func FindLatestVersion(index *IndexFile, chartName string) (string, error) {
	versions, ok := index.Entries[chartName]
	if !ok {
		return "", fmt.Errorf("chart %q not found in index", chartName)
	}

	var latest *semver.Version
	var latestStr string

	for _, v := range versions {
		sv, err := semver.NewVersion(v.Version)
		if err != nil {
			continue
		}
		if latest == nil || sv.GreaterThan(latest) {
			latest = sv
			latestStr = v.Version
		}
	}

	if latest == nil {
		return "", fmt.Errorf("no valid semver versions found for chart %q", chartName)
	}
	return latestStr, nil
}
