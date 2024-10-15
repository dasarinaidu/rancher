package workloads

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/rancher/rancher/tests/v2/actions/workloads/pods"
	"github.com/rancher/shepherd/clients/rancher"
	"github.com/rancher/shepherd/extensions/charts"
	"github.com/rancher/shepherd/extensions/kubectl"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rancher/shepherd/pkg/wrangler"
)

const (
	revisionAnnotation = "deployment.kubernetes.io/revision"
)

func validateDeploymentUpgrade(t *testing.T, client *rancher.Client, clusterName string, namespaceName string, appv1Deployment *appv1.Deployment, expectedRevision string, image string, expectedReplicas int) {
	log.Info("Waiting deployment comes up active")
	err := charts.WatchAndWaitDeployments(client, clusterName, namespaceName, metav1.ListOptions{
		FieldSelector: "metadata.name=" + appv1Deployment.Name,
	})
	require.NoError(t, err)

	log.Info("Waiting for all pods to be running")
	err = pods.WatchAndWaitPodContainerRunning(client, clusterName, namespaceName, appv1Deployment)
	require.NoError(t, err)

	log.Infof("Verifying rollout history by revision %s", expectedRevision)
	err = verifyDeploymentAgainstRolloutHistory(client, clusterName, namespaceName, appv1Deployment.Name, expectedRevision)
	require.NoError(t, err)

	log.Infof("Counting all pods running by image %s", image)
	countPods, err := pods.CountPodContainerRunningByImage(client, clusterName, namespaceName, image)
	require.NoError(t, err)
	require.Equal(t, expectedReplicas, countPods)
}

func validateDeploymentScale(t *testing.T, client *rancher.Client, clusterName string, namespaceName string, scaleDeployment *appv1.Deployment, image string, expectedReplicas int) {
	log.Info("Waiting deployment comes up active")
	err := charts.WatchAndWaitDeployments(client, clusterName, namespaceName, metav1.ListOptions{
		FieldSelector: "metadata.name=" + scaleDeployment.Name,
	})
	require.NoError(t, err)

	log.Info("Waiting for all pods to be running")
	err = pods.WatchAndWaitPodContainerRunning(client, clusterName, namespaceName, scaleDeployment)
	require.NoError(t, err)

	log.Infof("Counting all pods running by image %s", image)
	countPods, err := pods.CountPodContainerRunningByImage(client, clusterName, namespaceName, image)
	require.NoError(t, err)
	require.Equal(t, expectedReplicas, countPods)
}

func rollbackDeployment(client *rancher.Client, clusterID, namespaceName string, deploymentName string, revision int) (string, error) {
	deploymentCmd := fmt.Sprintf("deployment.apps/%s", deploymentName)
	revisionCmd := fmt.Sprintf("--to-revision=%s", strconv.Itoa(revision))
	execCmd := []string{"kubectl", "rollout", "undo", "-n", namespaceName, deploymentCmd, revisionCmd}
	logCmd, err := kubectl.Command(client, nil, clusterID, execCmd, "")
	return logCmd, err
}

func verifyDeploymentAgainstRolloutHistory(client *rancher.Client, clusterID, namespaceName string, deploymentName string, expectedRevision string) error {
	var wranglerContext *wrangler.Context
	var err error

	err = charts.WatchAndWaitDeployments(client, clusterID, namespaceName, metav1.ListOptions{
		FieldSelector: "metadata.name=" + deploymentName,
	})
	if err != nil {
		return err
	}

	wranglerContext = client.WranglerContext
	if clusterID != "local" {
		wranglerContext, err = client.WranglerContext.DownStreamClusterWranglerContext(clusterID)
		if err != nil {
			return err
		}
	}

	latestDeployment, err := wranglerContext.Apps.Deployment().Get(namespaceName, deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if latestDeployment.ObjectMeta.Annotations == nil {
		return errors.New("revision empty")
	}

	revision := latestDeployment.ObjectMeta.Annotations[revisionAnnotation]

	if revision != expectedRevision {
		return errors.New("revision not found")
	}

	return nil
}

func verifyOrchestrationStatus(client *rancher.Client, clusterID, namespaceName string, deploymentName string, isPaused bool) error {
	var wranglerContext *wrangler.Context
	var err error

	err = charts.WatchAndWaitDeployments(client, clusterID, namespaceName, metav1.ListOptions{
		FieldSelector: "metadata.name=" + deploymentName,
	})
	if err != nil {
		return err
	}

	wranglerContext = client.WranglerContext
	if clusterID != "local" {
		wranglerContext, err = client.WranglerContext.DownStreamClusterWranglerContext(clusterID)
		if err != nil {
			return err
		}
	}

	latestDeployment, err := wranglerContext.Apps.Deployment().Get(namespaceName, deploymentName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if isPaused && !latestDeployment.Spec.Paused {
		return errors.New("the orchestration is active")
	}

	if !isPaused && latestDeployment.Spec.Paused {
		return errors.New("the orchestration is paused")
	}

	return nil
}