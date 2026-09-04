package main

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func metaObject(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}
}