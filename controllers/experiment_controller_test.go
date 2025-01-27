package controllers

import (
	"context"
	"fmt"
	hackathonv1 "github.com/kaiyuanshe/cloudengine/api/v1"
	"github.com/kaiyuanshe/cloudengine/pkg/experiment"
	"github.com/kaiyuanshe/cloudengine/pkg/utils/k8stools"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var _ = Describe("test-experiment-reconcile", func() {
	var (
		expr *hackathonv1.Experiment
		tpl  = &hackathonv1.Template{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-template-for-exper",
				Namespace: "default",
			},
			Data: hackathonv1.TemplateData{
				Type: hackathonv1.PodTemplateType,
				PodTemplate: &hackathonv1.PodTemplate{
					Image:   "busybox",
					Command: []string{"sh", "-c", "sleep 100000000"},
				},
			},
		}
	)

	BeforeEach(func() {
		By("init cluster and template")
		time.Sleep(5 * time.Millisecond)
		Expect(k8sClient.Create(context.TODO(), tpl)).ToNot(HaveOccurred())
		expr = &hackathonv1.Experiment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-expr",
				Namespace: "default",
			},
			Spec: hackathonv1.ExperimentSpec{
				Pause:       false,
				Template:    tpl.Name,
				ClusterName: k8stools.MetaClusterName,
			},
		}
	})

	Context("reconcile-experiment", func() {
		It("create new", func() {
			By("create new experiment cr")
			Expect(k8sClient.Create(context.TODO(), expr)).ToNot(HaveOccurred())

			timeout := 60
			interval := 3
			created := &hackathonv1.Experiment{}
			Eventually(func() hackathonv1.ExperimentEnvStatus {
				Expect(k8sClient.Get(context.TODO(), types.NamespacedName{
					Namespace: expr.Namespace,
					Name:      expr.Name,
				}, created)).ToNot(HaveOccurred())
				return created.Status.Status
			}, timeout, interval).Should(Equal(hackathonv1.ExperimentRunning))

			podList := &corev1.PodList{}
			selector := labels.NewSelector()
			match, _ := labels.NewRequirement(experiment.LabelKeyExperimentName, selection.Equals, []string{expr.Name})
			selector = selector.Add(*match)
			Expect(k8sClient.List(context.TODO(), podList, client.MatchingLabelsSelector{Selector: selector}))

			Expect(len(podList.Items)).Should(Equal(1))
			envPod := podList.Items[0]
			Expect(k8stools.IsPodReady(&envPod)).Should(BeTrue())
		})
	})
	AfterEach(func() {
		_ = k8sClient.Delete(context.TODO(), expr)
		_ = k8sClient.Delete(context.TODO(), tpl)

		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pv-%s", expr.Name),
				Namespace: "default",
			},
		}
		_ = k8sClient.Delete(context.TODO(), pv)
	})
})
