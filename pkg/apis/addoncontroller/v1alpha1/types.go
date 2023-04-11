/*
Copyright 2017 The Kubernetes Authors.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Addon is a specification for a Addon resource
type Addon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AddonSpec   `json:"spec"`
	Status AddonStatus `json:"status"`
}

// AddonSpec is the spec for a Addon resource
type AddonSpec struct {
	Versions    []AddonVersion `json:"versions"`
	Description string         `json:"description"`
}

// 代表每一个插件在当前region所支持的版本
type AddonVersion struct {
	Version   string `json:"version"`
	Changelog string `json:"changelog"`
	Visible   bool   `json:"visible"`

	Client ClientSpec `json:"client"`
}

// 启动一个自研client，该client封装了helm命令行并以API形式进行暴露
type ClientSpec struct {
	Name    string `json:"name"`
	Image   string `json:"image"`
	Replica int    `json:"replica"`
}

// AddonStatus is the status for a Addon resource
type AddonStatus struct {
	// 暂时不需要定义status子资源
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AddonList is a list of Addon resources
type AddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Addon `json:"items"`
}
