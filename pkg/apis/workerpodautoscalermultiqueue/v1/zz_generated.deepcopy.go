//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2019 Practo Authors.

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

// Code generated by deepcopy-gen. DO NOT EDIT.

package v1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Queue) DeepCopyInto(out *Queue) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Queue.
func (in *Queue) DeepCopy() *Queue {
	if in == nil {
		return nil
	}
	out := new(Queue)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *WorkerPodAutoScalerMultiQueue) DeepCopyInto(out *WorkerPodAutoScalerMultiQueue) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new WorkerPodAutoScalerMultiQueue.
func (in *WorkerPodAutoScalerMultiQueue) DeepCopy() *WorkerPodAutoScalerMultiQueue {
	if in == nil {
		return nil
	}
	out := new(WorkerPodAutoScalerMultiQueue)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *WorkerPodAutoScalerMultiQueue) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *WorkerPodAutoScalerMultiQueueList) DeepCopyInto(out *WorkerPodAutoScalerMultiQueueList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]WorkerPodAutoScalerMultiQueue, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new WorkerPodAutoScalerMultiQueueList.
func (in *WorkerPodAutoScalerMultiQueueList) DeepCopy() *WorkerPodAutoScalerMultiQueueList {
	if in == nil {
		return nil
	}
	out := new(WorkerPodAutoScalerMultiQueueList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *WorkerPodAutoScalerMultiQueueList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *WorkerPodAutoScalerMultiQueueSpec) DeepCopyInto(out *WorkerPodAutoScalerMultiQueueSpec) {
	*out = *in
	if in.MinReplicas != nil {
		in, out := &in.MinReplicas, &out.MinReplicas
		*out = new(int32)
		**out = **in
	}
	if in.MaxReplicas != nil {
		in, out := &in.MaxReplicas, &out.MaxReplicas
		*out = new(int32)
		**out = **in
	}
	if in.MaxDisruption != nil {
		in, out := &in.MaxDisruption, &out.MaxDisruption
		*out = new(string)
		**out = **in
	}
	if in.Queues != nil {
		in, out := &in.Queues, &out.Queues
		*out = make([]Queue, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new WorkerPodAutoScalerMultiQueueSpec.
func (in *WorkerPodAutoScalerMultiQueueSpec) DeepCopy() *WorkerPodAutoScalerMultiQueueSpec {
	if in == nil {
		return nil
	}
	out := new(WorkerPodAutoScalerMultiQueueSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *WorkerPodAutoScalerMultiQueueStatus) DeepCopyInto(out *WorkerPodAutoScalerMultiQueueStatus) {
	*out = *in
	if in.LastScaleTime != nil {
		in, out := &in.LastScaleTime, &out.LastScaleTime
		*out = (*in).DeepCopy()
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new WorkerPodAutoScalerMultiQueueStatus.
func (in *WorkerPodAutoScalerMultiQueueStatus) DeepCopy() *WorkerPodAutoScalerMultiQueueStatus {
	if in == nil {
		return nil
	}
	out := new(WorkerPodAutoScalerMultiQueueStatus)
	in.DeepCopyInto(out)
	return out
}