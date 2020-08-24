/*
Copyright 2019 Bloomberg Finance LP.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

var (
	expectedBackupRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo-back", Namespace: "default"}}
)

func TestPodUpgradeOrdering(t *testing.T) {
	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: solr.SolrCloudSpec{
			SolrAddressability: solr.SolrAddressabilityOptions{
				PodPort: 2000,
			},
		},
	}

	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-0"},
			Spec:       corev1.PodSpec{},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
			Spec:       corev1.PodSpec{},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-2"},
			Spec:       corev1.PodSpec{},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-3"},
			Spec:       corev1.PodSpec{},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-4"},
			Spec:       corev1.PodSpec{},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-5"},
			Spec:       corev1.PodSpec{},
		},
	}

	nodeMap := map[string]SolrNodeContents{
		SolrNodeName(solrCloud, pods[0]): {
			nodeName:       SolrNodeName(solrCloud, pods[0]),
			leaders:        4,
			overseerLeader: true,
			live:           true,
		},
		SolrNodeName(solrCloud, pods[1]): {
			nodeName:       SolrNodeName(solrCloud, pods[1]),
			leaders:        8,
			overseerLeader: false,
			live:           false,
		},
		SolrNodeName(solrCloud, pods[2]): {
			nodeName:       SolrNodeName(solrCloud, pods[2]),
			leaders:        0,
			overseerLeader: false,
			live:           true,
		},
		SolrNodeName(solrCloud, pods[3]): {
			nodeName:       SolrNodeName(solrCloud, pods[3]),
			leaders:        3,
			overseerLeader: false,
			live:           true,
		},
		SolrNodeName(solrCloud, pods[4]): {
			nodeName:       SolrNodeName(solrCloud, pods[4]),
			leaders:        10,
			overseerLeader: false,
			live:           true,
		},
		SolrNodeName(solrCloud, pods[5]): {
			nodeName:       SolrNodeName(solrCloud, pods[5]),
			leaders:        3,
			overseerLeader: false,
			live:           true,
		},
	}

	expectedOrdering := []string{"pod-2", "pod-3", "pod-5", "pod-1", "pod-4", "pod-0"}

	sortNodePodsBySafety(pods, nodeMap, solrCloud)
	foundOrdering := make([]string, len(pods))
	for i, pod := range pods {
		foundOrdering[i] = pod.Name
	}
	assert.EqualValues(t, expectedOrdering, foundOrdering, "Ordering of pods not correct.")
}
