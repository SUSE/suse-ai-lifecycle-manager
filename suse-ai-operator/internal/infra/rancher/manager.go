package rancher

import (
	"github.com/SUSE/suse-ai-operator/internal/infra/helm"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Manager struct {
	client     client.Client
	scheme     *runtime.Scheme
	indexCache *helm.IndexCache
}

func NewManager(c client.Client, s *runtime.Scheme) *Manager {
	return &Manager{client: c, scheme: s, indexCache: helm.NewIndexCache()}
}
