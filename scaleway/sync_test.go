/*
Copyright 2018 Scaleway

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

package scaleway

import (
	"testing"
	//"k8s.io/client-go/kubernetes/fake"
)

func TestSyncController_SyncLBTags(t *testing.T) {
	//clientset := fake.NewSimpleClientset()

}

func TestSyncController_SyncNodeTags(t *testing.T) {
	//clientset := fake.NewSimpleClientset()

}

func TestTagTaintParser(t *testing.T) {
	tagsTest := map[string][3]string{
		"taint=":                    {"", "", ""},
		"taint=word=word:NoExecute": {"k8s.scaleway.com/word", "word", "NoExecute"},
		"taint=underscore_word=underscore_word:NoSchedule":                {"k8s.scaleway.com/underscore_word", "underscore_word", "NoSchedule"},
		"taint=dash-word=dash-word:PreferNoSchedule":                      {"k8s.scaleway.com/dash-word", "dash-word", "PreferNoSchedule"},
		"taint=dash-word=dash-word:foo":                                   {"", "", ""},
		"taint=dash-word=dash-word":                                       {"", "", ""},
		"taint=noprefix=dash-word=dash-word:PreferNoSchedule":             {"dash-word", "dash-word", "PreferNoSchedule"},
		"taint=noprefix=node.k8s.io/dash-word=dash-word:PreferNoSchedule": {"node.k8s.io/dash-word", "dash-word", "PreferNoSchedule"},
		"startup-taint=value":                                             {"", ""},
	}
	for tag, expected := range tagsTest {
		t.Run(tag, func(t *testing.T) {
			if key, value, effect := tagTaintParser(tag); key != expected[0] || value != expected[1] || string(effect) != expected[2] {
				t.Errorf("tagLabelParser(\"%s\") got %s, %s, %s expected %s, %s, %s", tag, key, value, effect, expected[0], expected[1], expected[2])
			}
		})
	}

}

func TestTagLabelParser(t *testing.T) {
	tagsTest := map[string][2]string{
		"":                      {"", ""},
		"word":                  {"k8s.scaleway.com/word", ""},
		"noprefix=word":         {"word", ""},
		"underscore_word":       {"k8s.scaleway.com/underscore_word", ""},
		"dash-word":             {"k8s.scaleway.com/dash-word", ""},
		"equal1=value":          {"k8s.scaleway.com/equal1", "value"},
		"noprefix=equal1=value": {"equal1", "value"},
		"noprefix=node.kubernetes.io/role=myrole": {"node.kubernetes.io/role", "myrole"},
		"equal2 = value":        {"k8s.scaleway.com/equal2", "value"},
		"equal3 =value":         {"k8s.scaleway.com/equal3", "value"},
		"equal4= value":         {"k8s.scaleway.com/equal4", "value"},
		"equal5==value":         {"k8s.scaleway.com/equal5", "value"},
		"equal6= =value":        {"k8s.scaleway.com/equal6", "value"},
		"colon1:value":          {"k8s.scaleway.com/colon1", "value"},
		"noprefix=colon1:value": {"colon1", "value"},
		"colon2 : value":        {"k8s.scaleway.com/colon2", "value"},
		"colon3 :value":         {"k8s.scaleway.com/colon3", "value"},
		"colon4: value":         {"k8s.scaleway.com/colon4", "value"},
		"colon5::value":         {"k8s.scaleway.com/colon5", "value"},
		"colon6: :value":        {"k8s.scaleway.com/colon6", "value"},
		"space word":            {"k8s.scaleway.com/space", "word"},
		"two space word":        {"k8s.scaleway.com/two", "space"},
		"key=accentué":          {"", ""},
		"accentué=value":        {"", ""},
		"word=invalid/dash":     {"", ""},
		"dash/word":             {"", ""},
		"dash/equal=value":      {"", ""},
	}

	for tag, expected := range tagsTest {
		t.Run(tag, func(t *testing.T) {
			if key, value := tagLabelParser(tag); key != expected[0] || value != expected[1] {
				t.Errorf("tagLabelParser(\"%s\") got %s, %s expected %s, %s", tag, key, value, expected[0], expected[1])
			}
		})
	}
}
