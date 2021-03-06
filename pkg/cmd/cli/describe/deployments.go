package describe

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/openshift/origin/pkg/api/graph"

	kapi "k8s.io/kubernetes/pkg/api"
	kerrors "k8s.io/kubernetes/pkg/api/errors"
	kclient "k8s.io/kubernetes/pkg/client/unversioned"
	kctl "k8s.io/kubernetes/pkg/kubectl"
	"k8s.io/kubernetes/pkg/labels"

	kubegraph "github.com/openshift/origin/pkg/api/kubegraph/nodes"
	"github.com/openshift/origin/pkg/client"
	deployapi "github.com/openshift/origin/pkg/deploy/api"
	deployedges "github.com/openshift/origin/pkg/deploy/graph"
	deploygraph "github.com/openshift/origin/pkg/deploy/graph/nodes"
	deployutil "github.com/openshift/origin/pkg/deploy/util"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

// DeploymentConfigDescriber generates information about a DeploymentConfig
type DeploymentConfigDescriber struct {
	client deploymentDescriberClient
}

type deploymentDescriberClient interface {
	getDeploymentConfig(namespace, name string) (*deployapi.DeploymentConfig, error)
	getDeployment(namespace, name string) (*kapi.ReplicationController, error)
	listDeployments(namespace string, selector labels.Selector) (*kapi.ReplicationControllerList, error)
	listPods(namespace string, selector labels.Selector) (*kapi.PodList, error)
	listEvents(deploymentConfig *deployapi.DeploymentConfig) (*kapi.EventList, error)
}

type genericDeploymentDescriberClient struct {
	getDeploymentConfigFunc func(namespace, name string) (*deployapi.DeploymentConfig, error)
	getDeploymentFunc       func(namespace, name string) (*kapi.ReplicationController, error)
	listDeploymentsFunc     func(namespace string, selector labels.Selector) (*kapi.ReplicationControllerList, error)
	listPodsFunc            func(namespace string, selector labels.Selector) (*kapi.PodList, error)
	listEventsFunc          func(deploymentConfig *deployapi.DeploymentConfig) (*kapi.EventList, error)
}

func (c *genericDeploymentDescriberClient) getDeploymentConfig(namespace, name string) (*deployapi.DeploymentConfig, error) {
	return c.getDeploymentConfigFunc(namespace, name)
}

func (c *genericDeploymentDescriberClient) getDeployment(namespace, name string) (*kapi.ReplicationController, error) {
	return c.getDeploymentFunc(namespace, name)
}

func (c *genericDeploymentDescriberClient) listDeployments(namespace string, selector labels.Selector) (*kapi.ReplicationControllerList, error) {
	return c.listDeploymentsFunc(namespace, selector)
}

func (c *genericDeploymentDescriberClient) listPods(namespace string, selector labels.Selector) (*kapi.PodList, error) {
	return c.listPodsFunc(namespace, selector)
}

func (c *genericDeploymentDescriberClient) listEvents(deploymentConfig *deployapi.DeploymentConfig) (*kapi.EventList, error) {
	return c.listEventsFunc(deploymentConfig)
}

// NewDeploymentConfigDescriberForConfig returns a new DeploymentConfigDescriber
// for a DeploymentConfig
func NewDeploymentConfigDescriberForConfig(client client.Interface, kclient kclient.Interface, config *deployapi.DeploymentConfig) *DeploymentConfigDescriber {
	return &DeploymentConfigDescriber{
		client: &genericDeploymentDescriberClient{
			getDeploymentConfigFunc: func(namespace, name string) (*deployapi.DeploymentConfig, error) {
				return config, nil
			},
			getDeploymentFunc: func(namespace, name string) (*kapi.ReplicationController, error) {
				return nil, kerrors.NewNotFound(kapi.Resource("replicatoncontroller"), name)
			},
			listDeploymentsFunc: func(namespace string, selector labels.Selector) (*kapi.ReplicationControllerList, error) {
				return nil, kerrors.NewNotFound(kapi.Resource("replicationcontrollerlist"), fmt.Sprintf("%v", selector))
			},
			listPodsFunc: func(namespace string, selector labels.Selector) (*kapi.PodList, error) {
				return nil, kerrors.NewNotFound(kapi.Resource("podlist"), fmt.Sprintf("%v", selector))
			},
			listEventsFunc: func(deploymentConfig *deployapi.DeploymentConfig) (*kapi.EventList, error) {
				return kclient.Events(config.Namespace).Search(config)
			},
		},
	}
}

// NewDeploymentConfigDescriber returns a new DeploymentConfigDescriber
func NewDeploymentConfigDescriber(client client.Interface, kclient kclient.Interface) *DeploymentConfigDescriber {
	return &DeploymentConfigDescriber{
		client: &genericDeploymentDescriberClient{
			getDeploymentConfigFunc: func(namespace, name string) (*deployapi.DeploymentConfig, error) {
				return client.DeploymentConfigs(namespace).Get(name)
			},
			getDeploymentFunc: func(namespace, name string) (*kapi.ReplicationController, error) {
				return kclient.ReplicationControllers(namespace).Get(name)
			},
			listDeploymentsFunc: func(namespace string, selector labels.Selector) (*kapi.ReplicationControllerList, error) {
				return kclient.ReplicationControllers(namespace).List(kapi.ListOptions{LabelSelector: selector})
			},
			listPodsFunc: func(namespace string, selector labels.Selector) (*kapi.PodList, error) {
				return kclient.Pods(namespace).List(kapi.ListOptions{LabelSelector: selector})
			},
			listEventsFunc: func(deploymentConfig *deployapi.DeploymentConfig) (*kapi.EventList, error) {
				return kclient.Events(deploymentConfig.Namespace).Search(deploymentConfig)
			},
		},
	}
}

// Describe returns the description of a DeploymentConfig
func (d *DeploymentConfigDescriber) Describe(namespace, name string) (string, error) {
	deploymentConfig, err := d.client.getDeploymentConfig(namespace, name)
	if err != nil {
		return "", err
	}
	events, err := d.client.listEvents(deploymentConfig)
	if err != nil {
		return "", err
	}

	return tabbedString(func(out *tabwriter.Writer) error {
		formatMeta(out, deploymentConfig.ObjectMeta)

		if deploymentConfig.Status.LatestVersion == 0 {
			formatString(out, "Latest Version", "Not deployed")
		} else {
			formatString(out, "Latest Version", strconv.Itoa(deploymentConfig.Status.LatestVersion))
		}

		printDeploymentConfigSpec(deploymentConfig.Spec, out)
		fmt.Fprintln(out)

		if deploymentConfig.Status.Details != nil && len(deploymentConfig.Status.Details.Message) > 0 {
			fmt.Fprintf(out, "Warning:\t%s\n", deploymentConfig.Status.Details.Message)
		}
		deploymentName := deployutil.LatestDeploymentNameForConfig(deploymentConfig)
		deployment, err := d.client.getDeployment(namespace, deploymentName)
		if err != nil {
			if kerrors.IsNotFound(err) {
				formatString(out, "Latest Deployment", "<none>")
			} else {
				formatString(out, "Latest Deployment", fmt.Sprintf("error: %v", err))
			}
		} else {
			header := fmt.Sprintf("Deployment #%d (latest)", deployutil.DeploymentVersionFor(deployment))
			printDeploymentRc(deployment, d.client, out, header, true)
		}
		deploymentsHistory, err := d.client.listDeployments(namespace, labels.Everything())
		if err == nil {
			sorted := rcSorter{}
			sorted = append(sorted, deploymentsHistory.Items...)
			sort.Sort(sorted)
			for _, item := range sorted {
				if item.Name != deploymentName && deploymentConfig.Name == deployutil.DeploymentConfigNameFor(&item) {
					header := fmt.Sprintf("Deployment #%d", deployutil.DeploymentVersionFor(&item))
					printDeploymentRc(&item, d.client, out, header, false)
				}
			}
		}

		if events != nil {
			fmt.Fprintln(out)
			kctl.DescribeEvents(events, out)
		}
		return nil
	})
}

func printStrategy(strategy deployapi.DeploymentStrategy, w *tabwriter.Writer) {
	switch strategy.Type {
	case deployapi.DeploymentStrategyTypeRecreate:
		if strategy.RecreateParams != nil {
			pre := strategy.RecreateParams.Pre
			mid := strategy.RecreateParams.Mid
			post := strategy.RecreateParams.Post
			if pre != nil {
				printHook("Pre-deployment", pre, w)
			}
			if mid != nil {
				printHook("Mid-deployment", mid, w)
			}
			if post != nil {
				printHook("Post-deployment", post, w)
			}
		}
	case deployapi.DeploymentStrategyTypeRolling:
		if strategy.RollingParams != nil {
			pre := strategy.RollingParams.Pre
			post := strategy.RollingParams.Post
			if pre != nil {
				printHook("Pre-deployment", pre, w)
			}
			if post != nil {
				printHook("Post-deployment", post, w)
			}
		}
	case deployapi.DeploymentStrategyTypeCustom:
		fmt.Fprintf(w, "\t  Image:\t%s\n", strategy.CustomParams.Image)

		if len(strategy.CustomParams.Environment) > 0 {
			fmt.Fprintf(w, "\t  Environment:\t%s\n", formatLabels(convertEnv(strategy.CustomParams.Environment)))
		}

		if len(strategy.CustomParams.Command) > 0 {
			fmt.Fprintf(w, "\t  Command:\t%v\n", strings.Join(strategy.CustomParams.Command, " "))
		}
	}
}

func printHook(prefix string, hook *deployapi.LifecycleHook, w io.Writer) {
	if hook.ExecNewPod != nil {
		fmt.Fprintf(w, "\t  %s hook (pod type, failure policy: %s):\n", prefix, hook.FailurePolicy)
		fmt.Fprintf(w, "\t    Container:\t%s\n", hook.ExecNewPod.ContainerName)
		fmt.Fprintf(w, "\t    Command:\t%v\n", strings.Join(hook.ExecNewPod.Command, " "))
		fmt.Fprintf(w, "\t    Env:\t%s\n", formatLabels(convertEnv(hook.ExecNewPod.Env)))
	}
}

func printTriggers(triggers []deployapi.DeploymentTriggerPolicy, w *tabwriter.Writer) {
	if len(triggers) == 0 {
		formatString(w, "Triggers", "<none>")
		return
	}

	labels := []string{}

	for _, t := range triggers {
		switch t.Type {
		case deployapi.DeploymentTriggerOnConfigChange:
			labels = append(labels, "Config")
		case deployapi.DeploymentTriggerOnImageChange:
			if len(t.ImageChangeParams.From.Name) > 0 {
				name, tag, _ := imageapi.SplitImageStreamTag(t.ImageChangeParams.From.Name)
				labels = append(labels, fmt.Sprintf("Image(%s@%s, auto=%v)", name, tag, t.ImageChangeParams.Automatic))
			}
		}
	}

	desc := strings.Join(labels, ", ")
	formatString(w, "Triggers", desc)
}

func printDeploymentConfigSpec(spec deployapi.DeploymentConfigSpec, w *tabwriter.Writer) error {
	// Selector
	formatString(w, "Selector", formatLabels(spec.Selector))

	// Replicas
	test := ""
	if spec.Test {
		test = " (test, will be scaled down between deployments)"
	}
	formatString(w, "Replicas", fmt.Sprintf("%d%s", spec.Replicas, test))

	// Triggers
	printTriggers(spec.Triggers, w)

	// Strategy
	formatString(w, "Strategy", spec.Strategy.Type)
	printStrategy(spec.Strategy, w)

	// Pod template
	fmt.Fprintf(w, "Template:\n")
	kctl.DescribePodTemplate(spec.Template, w)

	return nil
}

func printDeploymentRc(deployment *kapi.ReplicationController, client deploymentDescriberClient, w io.Writer, header string, verbose bool) error {
	if len(header) > 0 {
		fmt.Fprintf(w, "%v:\n", header)
	}

	if verbose {
		fmt.Fprintf(w, "\tName:\t%s\n", deployment.Name)
	}
	timeAt := strings.ToLower(formatRelativeTime(deployment.CreationTimestamp.Time))
	fmt.Fprintf(w, "\tCreated:\t%s ago\n", timeAt)
	fmt.Fprintf(w, "\tStatus:\t%s\n", deployutil.DeploymentStatusFor(deployment))
	fmt.Fprintf(w, "\tReplicas:\t%d current / %d desired\n", deployment.Status.Replicas, deployment.Spec.Replicas)

	if verbose {
		fmt.Fprintf(w, "\tSelector:\t%s\n", formatLabels(deployment.Spec.Selector))
		fmt.Fprintf(w, "\tLabels:\t%s\n", formatLabels(deployment.Labels))
		running, waiting, succeeded, failed, err := getPodStatusForDeployment(deployment, client)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "\tPods Status:\t%d Running / %d Waiting / %d Succeeded / %d Failed\n", running, waiting, succeeded, failed)
	}

	return nil
}

func getPodStatusForDeployment(deployment *kapi.ReplicationController, client deploymentDescriberClient) (running, waiting, succeeded, failed int, err error) {
	rcPods, err := client.listPods(deployment.Namespace, labels.SelectorFromSet(deployment.Spec.Selector))
	if err != nil {
		return
	}
	for _, pod := range rcPods.Items {
		switch pod.Status.Phase {
		case kapi.PodRunning:
			running++
		case kapi.PodPending:
			waiting++
		case kapi.PodSucceeded:
			succeeded++
		case kapi.PodFailed:
			failed++
		}
	}
	return
}

type LatestDeploymentsDescriber struct {
	count  int
	client deploymentDescriberClient
}

// NewLatestDeploymentsDescriber lists the latest deployments limited to "count". In case count == -1, list back to the last successful.
func NewLatestDeploymentsDescriber(client client.Interface, kclient kclient.Interface, count int) *LatestDeploymentsDescriber {
	return &LatestDeploymentsDescriber{
		count: count,
		client: &genericDeploymentDescriberClient{
			getDeploymentConfigFunc: func(namespace, name string) (*deployapi.DeploymentConfig, error) {
				return client.DeploymentConfigs(namespace).Get(name)
			},
			getDeploymentFunc: func(namespace, name string) (*kapi.ReplicationController, error) {
				return kclient.ReplicationControllers(namespace).Get(name)
			},
			listDeploymentsFunc: func(namespace string, selector labels.Selector) (*kapi.ReplicationControllerList, error) {
				return kclient.ReplicationControllers(namespace).List(kapi.ListOptions{LabelSelector: selector})
			},
			listPodsFunc: func(namespace string, selector labels.Selector) (*kapi.PodList, error) {
				return kclient.Pods(namespace).List(kapi.ListOptions{LabelSelector: selector})
			},
			listEventsFunc: func(deploymentConfig *deployapi.DeploymentConfig) (*kapi.EventList, error) {
				return kclient.Events(deploymentConfig.Namespace).Search(deploymentConfig)
			},
		},
	}
}

// Describe returns the description of the latest deployments for a config
func (d *LatestDeploymentsDescriber) Describe(namespace, name string) (string, error) {
	var f formatter

	config, err := d.client.getDeploymentConfig(namespace, name)
	if err != nil {
		return "", err
	}

	var deployments []kapi.ReplicationController
	if d.count == -1 || d.count > 1 {
		list, err := d.client.listDeployments(namespace, labels.Everything())
		if err != nil && !kerrors.IsNotFound(err) {
			return "", err
		}
		deployments = list.Items
	} else {
		deploymentName := deployutil.LatestDeploymentNameForConfig(config)
		deployment, err := d.client.getDeployment(config.Namespace, deploymentName)
		if err != nil && !kerrors.IsNotFound(err) {
			return "", err
		}
		if deployment != nil {
			deployments = []kapi.ReplicationController{*deployment}
		}
	}

	g := graph.New()
	dcNode := deploygraph.EnsureDeploymentConfigNode(g, config)
	for i := range deployments {
		kubegraph.EnsureReplicationControllerNode(g, &deployments[i])
	}
	deployedges.AddTriggerEdges(g, dcNode)
	deployedges.AddDeploymentEdges(g, dcNode)
	activeDeployment, inactiveDeployments := deployedges.RelevantDeployments(g, dcNode)

	return tabbedString(func(out *tabwriter.Writer) error {
		descriptions := describeDeployments(f, dcNode, activeDeployment, inactiveDeployments, d.count)
		for i, description := range descriptions {
			descriptions[i] = fmt.Sprintf("%v %v", name, description)
		}
		printLines(out, "", 0, descriptions...)
		return nil
	})
}

type rcSorter []kapi.ReplicationController

func (s rcSorter) Len() int {
	return len(s)
}
func (s rcSorter) Less(i, j int) bool {
	return s[i].CreationTimestamp.Unix() > s[j].CreationTimestamp.Unix()
}
func (s rcSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
