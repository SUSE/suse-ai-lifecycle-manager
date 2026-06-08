package kubernetes

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func IsDeploymentReady(
	ctx context.Context,
	c client.Client,
	namespace, releaseName string,
	log logr.Logger,
) (bool, error) {

	var list appsv1.DeploymentList
	if err := c.List(
		ctx,
		&list,
		client.InNamespace(namespace),
		client.MatchingLabels{
			"app.kubernetes.io/instance": releaseName,
		},
	); err != nil {
		return false, err
	}

	if len(list.Items) == 0 {
		log.Info("No deployments found for release", "release", releaseName, "namespace", namespace)
		return false, nil
	}

	for _, d := range list.Items {
		desired := int32(1)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		if d.Status.ReadyReplicas < desired {
			log.Info("Deployment not ready",
				"deployment", d.Name,
				"readyReplicas", d.Status.ReadyReplicas,
				"desiredReplicas", desired,
				"availableReplicas", d.Status.AvailableReplicas,
				"unavailableReplicas", d.Status.UnavailableReplicas,
			)
			return false, nil
		}
	}

	log.Info("All deployments ready", "release", releaseName, "count", len(list.Items))
	return true, nil
}
