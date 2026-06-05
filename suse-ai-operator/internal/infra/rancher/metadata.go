package rancher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/exp/maps"

	"github.com/SUSE/suse-ai-operator/api/v1beta1"
	"github.com/SUSE/suse-ai-operator/internal/infra/helm"
	logging "github.com/SUSE/suse-ai-operator/internal/logging"
)

const (
	KeyDisplayName       = "catalog.cattle.io/display-name"
	KeyRancherVersion    = "catalog.cattle.io/rancher-version"
	KeyUIExtensionsRange = "catalog.cattle.io/ui-extensions-version"
)

func buildExtensionMetadata(
	ctx context.Context,
	indexCache *helm.IndexCache,
	repoURL string,
	ext *v1beta1.InstallAIExtension,
) (map[string]string, error) {

	extensionName := ext.Spec.Extension.Name
	version := ext.Spec.Extension.Version

	log := logging.FromContext(ctx, "rancher.metadata").
		WithValues(
			logging.KeyExtension, extensionName,
			logging.KeyVersion, version,
		)

	logging.Debug(log).Info("Resolving extension metadata from index")

	indexURLs, err := indexURLsForSource(ext, repoURL)
	if err != nil {
		return nil, err
	}

	index, err := getOrFetchIndex(ctx, indexCache, indexURLs)
	if err != nil {
		log.Error(err, "Failed to load Helm index")
		return nil, err
	}

	annotations, err := helm.FindAnnotations(index, extensionName, version)
	if err != nil {
		log.Error(err, "Failed to find chart annotations in index.yaml")
		return nil, err
	}

	indexMeta := filterSupportedMetadata(annotations)

	logging.Trace(log).Info(
		"Metadata extracted from index.yaml",
		"metadata", indexMeta,
	)

	userMeta := ext.Spec.Extension.Metadata
	if userMeta == nil {
		userMeta = map[string]string{}
	}

	final := mergeMetadata(indexMeta, userMeta, extensionName)

	logging.Debug(log).Info(
		"Final UIPlugin metadata resolved",
		"displayName", final[KeyDisplayName],
		"uiExtensionsVersion", final[KeyUIExtensionsRange],
	)

	// Return a clone to avoid accidental mutation
	return maps.Clone(final), nil
}

func indexURLsForSource(ext *v1beta1.InstallAIExtension, svcURL string) ([]string, error) {

	switch {
	case ext.Spec.Source.Helm != nil:
		return []string{
			fmt.Sprintf("%s/index.yaml", svcURL),
		}, nil

	case ext.Spec.Source.Git != nil:
		base := GitRawBaseURL(ext.Spec.Source.Git.Repo, ext.Spec.Source.Git.Branch)
		return []string{
			fmt.Sprintf("%s/index.yaml", base),
			fmt.Sprintf("%s/assets/index.yaml", base),
		}, nil

	default:
		return nil, fmt.Errorf("source must specify either helm or git")
	}
}

func normalizeGitHubRepo(repo string) string {
	repo = strings.TrimPrefix(repo, "https://")
	repo = strings.TrimPrefix(repo, "http://")
	repo = strings.TrimPrefix(repo, "github.com/")
	repo = strings.TrimSuffix(repo, ".git")
	return repo
}

func GitRawBaseURL(repo string, branch string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/refs/heads/%s", normalizeGitHubRepo(repo), branch)
}

func getOrFetchIndex(
	ctx context.Context,
	cache *helm.IndexCache,
	indexURLs []string,
) (*helm.IndexFile, error) {

	// Check cache for any of the URLs
	for _, url := range indexURLs {
		key := helm.IndexCacheKey{RepoURL: url}
		if entry, ok := cache.Get(key); ok {
			return entry.Index, nil
		}
	}

	// Try each URL in order
	var lastErr error
	for _, url := range indexURLs {
		index, err := helm.FetchIndex(url)
		if err != nil {
			lastErr = err
			continue
		}

		cache.Set(helm.IndexCacheKey{RepoURL: url}, &helm.IndexCacheEntry{
			Index:     index,
			FetchedAt: time.Now(),
		})

		return index, nil
	}

	return nil, fmt.Errorf("failed to fetch index.yaml from any URL: %w", lastErr)
}

func filterSupportedMetadata(
	annotations map[string]string,
) map[string]string {

	meta := map[string]string{}

	for _, key := range []string{
		KeyDisplayName,
		KeyRancherVersion,
		KeyUIExtensionsRange,
	} {
		if val, ok := annotations[key]; ok {
			meta[key] = val
		}
	}

	return meta
}

func mergeMetadata(
	indexMeta map[string]string,
	userMeta map[string]string,
	extensionName string,
) map[string]string {

	meta := maps.Clone(indexMeta)

	// User overrides always win
	for k, v := range userMeta {
		meta[k] = v
	}

	// Safe default
	if _, ok := meta[KeyDisplayName]; !ok {
		meta[KeyDisplayName] = extensionName
	}

	return meta
}
