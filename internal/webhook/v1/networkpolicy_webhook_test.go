/*
Copyright 2025.

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

package v1

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("NetworkPolicy Webhook", func() {
	var (
		obj        *networkingv1.NetworkPolicy
		oldObj     *networkingv1.NetworkPolicy
		validator  NetworkPolicyCustomValidator
		defaulter  NetworkPolicyCustomDefaulter
		ctx        context.Context
		fakeClient client.Client
		namespace  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "test-namespace"

		// Create a namespace for our tests
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		// Create a new scheme and register all the types we need
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(networkingv1.AddToScheme(scheme)).To(Succeed())
		Expect(certificatesv1.AddToScheme(scheme)).To(Succeed())

		// Initialize fake client with the namespace
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(ns).
			Build()

		// Initialize the validator with the fake client
		validator = NetworkPolicyCustomValidator{
			Client: fakeClient,
		}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")

		// Initialize the defaulter
		defaulter = NetworkPolicyCustomDefaulter{}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")

		// Create test NetworkPolicy objects
		obj = &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy",
				Namespace: namespace,
			},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{},
				Ingress: []networkingv1.NetworkPolicyIngressRule{
					{
						From: []networkingv1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app": "test",
									},
								},
							},
						},
					},
				},
			},
		}

		oldObj = obj.DeepCopy()
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
		// Clean up resources if needed
	})

	Context("When creating NetworkPolicy under Defaulting Webhook", func() {
		It("Should not modify the NetworkPolicy as no defaults are implemented", func() {
			By("Creating a copy of the original NetworkPolicy")
			original := obj.DeepCopy()

			By("Calling the Default method")
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the NetworkPolicy was not modified")
			Expect(obj).To(Equal(original))
		})
	})

	Context("When creating or updating NetworkPolicy under Validating Webhook", func() {
		It("Should deny creation if no approval exists", func() {
			By("Attempting to validate a NetworkPolicy without approval")
			warnings, err := validator.ValidateCreate(ctx, obj)

			By("Verifying the validation fails with appropriate error")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("NetworkPolicy has not been approved yet"))
			Expect(warnings).To(BeNil())

			By("Verifying a CSR was created")
			csrName := fmt.Sprintf("np-approval-%s-%s", namespace, obj.Name)
			csr := &certificatesv1.CertificateSigningRequest{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: csrName}, csr)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying CSR has correct labels and annotations")
			Expect(csr.Labels[LabelNetworkPolicyApproval]).To(Equal("true"))
			Expect(csr.Annotations["networkpolicy.webhook.io/name"]).To(Equal(obj.Name))
			Expect(csr.Annotations["networkpolicy.webhook.io/namespace"]).To(Equal(namespace))
			Expect(csr.Annotations).To(HaveKey(AnnotationApprovalHash))
		})

		It("Should allow creation if approval exists", func() {
			By("Generating a hash for the NetworkPolicy")
			hash, err := generateNetworkPolicyHash(obj)
			Expect(err).NotTo(HaveOccurred())

			By("Creating an approval secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("np-approval-%s-%s", namespace, obj.Name),
					Namespace: namespace,
					Labels: map[string]string{
						"networkpolicy.webhook.io/approval": "true",
						"networkpolicy.webhook.io/name":     obj.Name,
					},
					Annotations: map[string]string{
						"networkpolicy.webhook.io/csr-name":      "test-csr",
						"networkpolicy.webhook.io/approval-hash": hash,
						"networkpolicy.webhook.io/np-name":       obj.Name,
						"networkpolicy.webhook.io/np-namespace":  namespace,
					},
				},
				Type: SecretTypeNetworkPolicyApproval,
				Data: map[string][]byte{
					"hash":     []byte(hash),
					"tls-crt":  []byte("-----BEGIN CERTIFICATE-----\nMIIDazCCAlOgAwIBAgIUJsUmwbNpFG9X8P4Xg/fJ9UJa2BwwDQYJKoZIhvcNAQEL\nBQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM\nGEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMzA1MzEwMDAwMDBaFw0zMzA1\nMzEwMDAwMDBaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw\nHwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB\nAQUAA4IBDwAwggEKAoIBAQCqS5ivHGSUXQXf4YMhfI9g5k0WVJAkVBs7oTtgwDvw\nAT5YGCQZIzEHSU1h6g1QYk4nrqCKpV9jGJzoyYnAJkLYxBDLHN9xJLAAYJQXJXxU\nJGNLHQk+zGwz+tzJlvgYjzVQD4Jz+WXIcFzLzjIwYB+BS4LkAx/OoXrz3GVRcEQi\nQWHiVsUYKrVxqCRlvGzvhxWJQHGYmwYRCGgEmVZYJ9HJJYIvV+ygzGUJHJ7UFjUa\nG4QKj+QIjP9fFZrCrYh9GzJ3pOGIgT+8kXQNJo5X8MgNyj4gQlUYm0yCOxPIKN4w\nWlWUcFxiYgwN7K9oMYQXJzQKcLEYdYbOEJXJEwSdAgMBAAGjUzBRMB0GA1UdDgQW\nBBQHWYs6YAKZXgDciTdPzScCiaSLaTAfBgNVHSMEGDAWgBQHWYs6YAKZXgDciTdP\nzScCiaSLaTAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBVGwOZ\nC2AxHK/arOJ8B7QwHkTi3z3NAqF03hYMFKOWXCQ7GBUzxQLtQjPB9SSBR9fGLlGu\nTj9q+AJpvnApOFDbAO9RBIzDUqVQgGz6RFYMvnPFzJRHJi3qdWzKYjYOTYCvAZFz\nYKiBIIVwArOYzaQRV0GsaKSuCUVTCJXS6urHpFyZWYCnYQUYs8YyGIjqJRuLIEhk\nM2d+CJg9hGV/0UecgRQC9fhIYUwcLYiM2FMYw98uU5OoUarQyLHHGQjJnLOHxK5k\nfPSAH5i4jPXS8GYSZQBpgzI5egrj5+6RRTBjMKmnCLEz9jUFe3QUO9N9+jDzZf/G\nNPX6JQZ1R9wNv+E5\n-----END CERTIFICATE-----"),
					"csr-name": []byte("test-csr"),
				},
			}
			Expect(fakeClient.Create(ctx, secret)).To(Succeed())

			By("Validating the NetworkPolicy with existing approval")
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("Should deny update if hash doesn't match approval", func() {
			By("Creating an approval secret with a different hash")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("np-approval-%s-%s", namespace, obj.Name),
					Namespace: namespace,
					Labels: map[string]string{
						"networkpolicy.webhook.io/approval": "true",
						"networkpolicy.webhook.io/name":     obj.Name,
					},
					Annotations: map[string]string{
						"networkpolicy.webhook.io/csr-name":      "test-csr",
						"networkpolicy.webhook.io/approval-hash": "different-hash",
						"networkpolicy.webhook.io/np-name":       obj.Name,
						"networkpolicy.webhook.io/np-namespace":  namespace,
					},
				},
				Type: SecretTypeNetworkPolicyApproval,
				Data: map[string][]byte{
					"hash":     []byte("different-hash"),
					"tls-crt":  []byte("-----BEGIN CERTIFICATE-----\nMIIDazCCAlOgAwIBAgIUJsUmwbNpFG9X8P4Xg/fJ9UJa2BwwDQYJKoZIhvcNAQEL\nBQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM\nGEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMzA1MzEwMDAwMDBaFw0zMzA1\nMzEwMDAwMDBaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw\nHwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB\nAQUAA4IBDwAwggEKAoIBAQCqS5ivHGSUXQXf4YMhfI9g5k0WVJAkVBs7oTtgwDvw\nAT5YGCQZIzEHSU1h6g1QYk4nrqCKpV9jGJzoyYnAJkLYxBDLHN9xJLAAYJQXJXxU\nJGNLHQk+zGwz+tzJlvgYjzVQD4Jz+WXIcFzLzjIwYB+BS4LkAx/OoXrz3GVRcEQi\nQWHiVsUYKrVxqCRlvGzvhxWJQHGYmwYRCGgEmVZYJ9HJJYIvV+ygzGUJHJ7UFjUa\nG4QKj+QIjP9fFZrCrYh9GzJ3pOGIgT+8kXQNJo5X8MgNyj4gQlUYm0yCOxPIKN4w\nWlWUcFxiYgwN7K9oMYQXJzQKcLEYdYbOEJXJEwSdAgMBAAGjUzBRMB0GA1UdDgQW\nBBQHWYs6YAKZXgDciTdPzScCiaSLaTAfBgNVHSMEGDAWgBQHWYs6YAKZXgDciTdP\nzScCiaSLaTAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBVGwOZ\nC2AxHK/arOJ8B7QwHkTi3z3NAqF03hYMFKOWXCQ7GBUzxQLtQjPB9SSBR9fGLlGu\nTj9q+AJpvnApOFDbAO9RBIzDUqVQgGz6RFYMvnPFzJRHJi3qdWzKYjYOTYCvAZFz\nYKiBIIVwArOYzaQRV0GsaKSuCUVTCJXS6urHpFyZWYCnYQUYs8YyGIjqJRuLIEhk\nM2d+CJg9hGV/0UecgRQC9fhIYUwcLYiM2FMYw98uU5OoUarQyLHHGQjJnLOHxK5k\nfPSAH5i4jPXS8GYSZQBpgzI5egrj5+6RRTBjMKmnCLEz9jUFe3QUO9N9+jDzZf/G\nNPX6JQZ1R9wNv+E5\n-----END CERTIFICATE-----"),
					"csr-name": []byte("test-csr"),
				},
			}
			Expect(fakeClient.Create(ctx, secret)).To(Succeed())

			By("Modifying the NetworkPolicy")
			obj.Spec.Ingress[0].From[0].PodSelector.MatchLabels["app"] = "modified"

			By("Validating the update")
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("NetworkPolicy has not been approved yet"))
			Expect(warnings).To(BeNil())
		})

		It("Should allow deletion without approval check", func() {
			By("Validating deletion")
			warnings, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})
	})

	Context("When generating hash for NetworkPolicy", func() {
		It("Should generate consistent hash for same NetworkPolicy", func() {
			By("Generating hash for the NetworkPolicy")
			hash1, err := generateNetworkPolicyHash(obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash1).NotTo(BeEmpty())

			By("Generating hash again for the same NetworkPolicy")
			hash2, err := generateNetworkPolicyHash(obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash2).NotTo(BeEmpty())

			By("Verifying both hashes are identical")
			Expect(hash1).To(Equal(hash2))
		})

		It("Should generate different hash for modified NetworkPolicy", func() {
			By("Generating hash for the original NetworkPolicy")
			hash1, err := generateNetworkPolicyHash(obj)
			Expect(err).NotTo(HaveOccurred())

			By("Modifying the NetworkPolicy")
			modified := obj.DeepCopy()
			modified.Spec.Ingress[0].From[0].PodSelector.MatchLabels["app"] = "modified"

			By("Generating hash for the modified NetworkPolicy")
			hash2, err := generateNetworkPolicyHash(modified)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying hashes are different")
			Expect(hash1).NotTo(Equal(hash2))
		})
	})
})
