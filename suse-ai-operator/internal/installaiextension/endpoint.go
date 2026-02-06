package installaiextension

import (
	"fmt"
	"net/url"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

func EndpointFromGitRepo(
	repoURL, branch, pluginName, version string,
) (string, error) {

	if repoURL == "" || branch == "" || pluginName == "" || version == "" {
		return "", fmt.Errorf("repoURL, branch, pluginName and version must be set")
	}

	u, err := url.Parse(repoURL)
	if err != nil {
		return "", err
	}

	parts := strings.Split(strings.TrimSuffix(u.Path, ".git"), "/")
	if len(parts) < 3 {
		return "", fmt.Errorf("unexpected repo path: %s", u.Path)
	}

	return fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/extensions/%s/%s",
		parts[1],
		parts[2],
		branch,
		pluginName,
		version,
	), nil
}

func ServiceEndpoint(svc *corev1.Service) (name, namespace string, port int32, error error) {
	if svc == nil {
		return "", "", 0, fmt.Errorf("service is nil")
	}

	if len(svc.Spec.Ports) == 0 {
		return "", "", 0, fmt.Errorf("service %s has no ports", svc.Name)
	}

	return svc.Name, svc.Namespace, svc.Spec.Ports[0].Port, nil
}
