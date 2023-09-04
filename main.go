package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"
	clientconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	quotaLabel = "resourcequota.kubesphere.io/enable"
)

type config struct {
	nsSelector        string
	excludeNamespaces []string
	cpuLimit          string
	memLimit          string
	pvcSizeLimit      string
	quotaName         string
}

type quotaManager struct {
	k8sClient kubernetes.Interface
	conf      *config
}

func main() {
	klog.InitFlags(nil)
	var nsSelector string
	var excludeNSes string
	var cpuLimit string
	var memLimit string
	var pvcSizeLimit string
	var quotaName string
	flag.StringVar(&nsSelector, "namespace-selector", "", "namespace selector")
	flag.StringVar(&excludeNSes, "exclude-namespace", "kube-system,kubesphere-system", "comma separated excluded namespaces")
	flag.StringVar(&cpuLimit, "cpu-limit", "1000", "limits.cpu")
	flag.StringVar(&memLimit, "mem-limit", "1000Gi", "limits.memory")
	flag.StringVar(&pvcSizeLimit, "storage-limit", "1000Ti", "requests.storage")
	flag.StringVar(&quotaName, "resource-quota-name", "default-quota", "resource quota name")
	flag.Parse()

	conf := &config{
		nsSelector:        nsSelector,
		excludeNamespaces: strings.Split(excludeNSes, ","),
		cpuLimit:          cpuLimit,
		memLimit:          memLimit,
		pvcSizeLimit:      pvcSizeLimit,
		quotaName:         quotaName,
	}
	klog.Infoln("config", conf)
	ctx := context.Background()
	restConfig := clientconfig.GetConfigOrDie()
	k8sClient := kubernetes.NewForConfigOrDie(restConfig)
	manager := quotaManager{
		k8sClient: k8sClient,
		conf:      conf,
	}
	err := manager.do(ctx)
	if err != nil {
		klog.ErrorS(err, "do failed")
		os.Exit(1)
	}
}

func (q *quotaManager) do(ctx context.Context) error {
	version, err := q.k8sClient.Discovery().ServerVersion()
	if err != nil {
		return err
	}
	klog.Infoln("kubernetes version", version.String())

	nsSelector := labels.Everything()
	if len(q.conf.nsSelector) > 0 {
		nsSelector, err = labels.Parse(q.conf.nsSelector)
		if err != nil {
			return err
		}
	}

	nsWatch, err := q.k8sClient.CoreV1().Namespaces().Watch(ctx, v1.ListOptions{
		LabelSelector: nsSelector.String(),
	})
	if err != nil {
		return err
	}
	for e := range nsWatch.ResultChan() {
		klog.V(4).Infoln("got an event", e)
		if e.Type == watch.Error {
			return fmt.Errorf("watch namespace error: %+v", e)
		}
		if e.Type != watch.Added && e.Type != watch.Modified {
			continue
		}
		ns, ok := e.Object.(*corev1.Namespace)
		if !ok {
			continue
		}
		if slices.Contains(q.conf.excludeNamespaces, ns.Name) {
			continue
		}
		if ns.Status.Phase != corev1.NamespaceActive {
			continue
		}
		var quota *corev1.ResourceQuota
		quota, err = q.k8sClient.CoreV1().ResourceQuotas(ns.Name).Get(ctx, q.conf.quotaName, v1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				klog.ErrorS(err, "get resource quota failed", "namespace", ns.Name)
				continue
			}
			quota = &corev1.ResourceQuota{
				TypeMeta: v1.TypeMeta{
					Kind:       "ResourceQuota",
					APIVersion: corev1.SchemeGroupVersion.String(),
				},
				ObjectMeta: v1.ObjectMeta{
					Name:      q.conf.quotaName,
					Namespace: ns.Name,
					Labels: map[string]string{
						quotaLabel: "true",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						"limits.cpu":       resource.MustParse(q.conf.cpuLimit),
						"limits.memory":    resource.MustParse(q.conf.memLimit),
						"requests.storage": resource.MustParse(q.conf.pvcSizeLimit),
					},
				},
			}
			createQuotaFunc := func() error {
				_, err = q.k8sClient.CoreV1().ResourceQuotas(ns.Name).Create(ctx, quota, v1.CreateOptions{})
				return err
			}
			err = retry.RetryOnConflict(retry.DefaultRetry, createQuotaFunc)
			if err != nil {
				return err
			}
			klog.Infoln("create resource quota successfully", "namespace", ns.Name, "quota", quota)
		}
	}
	return nil
}
