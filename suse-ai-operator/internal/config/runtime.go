package config

import "os"

const DefaultExtensionNamespace = "cattle-ui-plugin-system"

func GetExtensionNamespace() string {
	if ns := os.Getenv("EXTENSION_NAMESPACE"); ns != "" {
		return ns
	}
	return DefaultExtensionNamespace
}
