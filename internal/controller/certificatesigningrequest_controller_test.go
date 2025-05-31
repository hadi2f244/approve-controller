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

package controller

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("CertificateSigningRequest Controller", func() {
	var (
		reconciler *CertificateSigningRequestReconciler
		fakeClient client.Client
		testLogger logr.Logger
		recorder   *record.FakeRecorder
		ctx        context.Context
		req        ctrl.Request
		csr        *certificatesv1.CertificateSigningRequest
		namespace  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testLogger = logf.Log.WithName("test")
		recorder = record.NewFakeRecorder(10)
		namespace = "test-namespace"

		// Create a namespace for our tests
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		// Initialize fake client with the namespace
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(ns).
			WithIndex(&corev1.Secret{}, "type", func(obj client.Object) []string {
				secret := obj.(*corev1.Secret)
				return []string{string(secret.Type)}
			}).
			Build()

		// Create the shared reconciler
		sharedReconciler := NewSharedReconciler(
			fakeClient,
			scheme.Scheme,
			fakeClient, // Using the same client for both client and apiReader
			testLogger,
			recorder,
		)

		// Create the CSR reconciler
		reconciler = &CertificateSigningRequestReconciler{
			SharedReconciler: sharedReconciler,
		}

		// Create a test CSR
		csr = &certificatesv1.CertificateSigningRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-csr",
				Labels: map[string]string{
					"networkpolicy.webhook.io/approval": "true",
				},
				Annotations: map[string]string{
					"networkpolicy.webhook.io/name":          "test-policy",
					"networkpolicy.webhook.io/namespace":     namespace,
					"networkpolicy.webhook.io/approval-hash": "test-hash-123",
				},
			},
			Spec: certificatesv1.CertificateSigningRequestSpec{
				Request: []byte("test-csr-data"),
				Usages: []certificatesv1.KeyUsage{
					certificatesv1.UsageDigitalSignature,
					certificatesv1.UsageKeyEncipherment,
				},
				SignerName: "kubernetes.io/kube-apiserver-client",
			},
		}

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name: csr.Name,
			},
		}
	})

	Context("When reconciling a CSR that doesn't exist", func() {
		It("should return without error", func() {
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})
	})

	Context("When reconciling a CSR without the NetworkPolicy approval label", func() {
		BeforeEach(func() {
			// Create a CSR without the label
			unlabeledCSR := csr.DeepCopy()
			unlabeledCSR.Labels = map[string]string{}
			Expect(fakeClient.Create(ctx, unlabeledCSR)).To(Succeed())
		})

		It("should ignore the CSR and return without error", func() {
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})
	})

	Context("When reconciling a CSR that is not yet approved", func() {
		BeforeEach(func() {
			// Create the CSR without approval
			Expect(fakeClient.Create(ctx, csr)).To(Succeed())
		})

		It("should not create a secret and return without error", func() {
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Check that no secret was created
			secretName := types.NamespacedName{
				Name:      "np-approval-test-namespace-test-policy",
				Namespace: namespace,
			}
			secret := &corev1.Secret{}
			err = fakeClient.Get(ctx, secretName, secret)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling an approved CSR with certificate data", func() {
		BeforeEach(func() {
			// Create the CSR with approval and certificate data
			approvedCSR := csr.DeepCopy()
			approvedCSR.Status = certificatesv1.CertificateSigningRequestStatus{
				Conditions: []certificatesv1.CertificateSigningRequestCondition{
					{
						Type:    certificatesv1.CertificateApproved,
						Status:  corev1.ConditionTrue,
						Reason:  "Approved",
						Message: "Approved by test",
					},
				},
				Certificate: []byte("test-certificate-data"),
			}
			Expect(fakeClient.Create(ctx, approvedCSR)).To(Succeed())
		})

		It("should create a secret with the certificate data", func() {
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Check that a secret was created
			secretName := types.NamespacedName{
				Name:      "np-approval-test-namespace-test-policy",
				Namespace: namespace,
			}
			secret := &corev1.Secret{}
			err = fakeClient.Get(ctx, secretName, secret)
			Expect(err).NotTo(HaveOccurred())

			// Verify secret contents
			Expect(secret.Type).To(Equal(corev1.SecretType("networkpolicy.webhook.io/approval")))
			Expect(secret.Data["hash"]).To(Equal([]byte("test-hash-123")))
			Expect(secret.Data["tls-crt"]).To(Equal([]byte("test-certificate-data")))
			Expect(secret.Data["csr-name"]).To(Equal([]byte("test-csr")))

			// Verify annotations
			Expect(secret.Annotations["networkpolicy.webhook.io/csr-name"]).To(Equal("test-csr"))
			Expect(secret.Annotations["networkpolicy.webhook.io/approval-hash"]).To(Equal("test-hash-123"))
			Expect(secret.Annotations["networkpolicy.webhook.io/np-name"]).To(Equal("test-policy"))
			Expect(secret.Annotations["networkpolicy.webhook.io/np-namespace"]).To(Equal(namespace))

			// Verify finalizer
			Expect(secret.Finalizers).To(ContainElement("networkpolicy.webhook.io/approval-protection"))
		})
	})

	Context("When reconciling an approved CSR with an existing secret", func() {
		BeforeEach(func() {
			// Create the CSR with approval and certificate data
			approvedCSR := csr.DeepCopy()
			approvedCSR.Status = certificatesv1.CertificateSigningRequestStatus{
				Conditions: []certificatesv1.CertificateSigningRequestCondition{
					{
						Type:    certificatesv1.CertificateApproved,
						Status:  corev1.ConditionTrue,
						Reason:  "Approved",
						Message: "Approved by test",
					},
				},
				Certificate: []byte("test-certificate-data"),
			}
			Expect(fakeClient.Create(ctx, approvedCSR)).To(Succeed())

			// Create an existing secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "np-approval-test-namespace-test-policy",
					Namespace: namespace,
					Labels: map[string]string{
						"networkpolicy.webhook.io/approval": "true",
						"networkpolicy.webhook.io/name":     "test-policy",
					},
					Annotations: map[string]string{
						"networkpolicy.webhook.io/csr-name":      "test-csr",
						"networkpolicy.webhook.io/approval-hash": "old-hash",
						"networkpolicy.webhook.io/np-name":       "test-policy",
						"networkpolicy.webhook.io/np-namespace":  namespace,
					},
				},
				Type: "networkpolicy.webhook.io/approval",
				Data: map[string][]byte{
					"hash":     []byte("old-hash"),
					"tls-crt":  []byte("old-certificate-data"),
					"csr-name": []byte("test-csr"),
				},
			}
			Expect(fakeClient.Create(ctx, secret)).To(Succeed())
		})

		It("should update the existing secret with new certificate data", func() {
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Check that the secret was updated
			secretName := types.NamespacedName{
				Name:      "np-approval-test-namespace-test-policy",
				Namespace: namespace,
			}
			secret := &corev1.Secret{}
			err = fakeClient.Get(ctx, secretName, secret)
			Expect(err).NotTo(HaveOccurred())

			// Verify updated secret contents
			Expect(secret.Data["hash"]).To(Equal([]byte("test-hash-123")))
			Expect(secret.Data["tls-crt"]).To(Equal([]byte("test-certificate-data")))
		})
	})

	Context("When cleaning up orphaned secrets", func() {
		BeforeEach(func() {
			// Create an orphaned secret (no corresponding CSR)
			orphanedSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "np-approval-test-namespace-orphaned",
					Namespace: namespace,
					Labels: map[string]string{
						"networkpolicy.webhook.io/approval": "true",
						"networkpolicy.webhook.io/name":     "orphaned",
					},
					Annotations: map[string]string{
						"networkpolicy.webhook.io/csr-name":      "non-existent-csr",
						"networkpolicy.webhook.io/approval-hash": "orphaned-hash",
						"networkpolicy.webhook.io/np-name":       "orphaned",
						"networkpolicy.webhook.io/np-namespace":  namespace,
					},
					Finalizers: []string{"networkpolicy-approval-protection"},
				},
				Type: "networkpolicy.webhook.io/approval",
				Data: map[string][]byte{
					"hash":     []byte("orphaned-hash"),
					"tls-crt":  []byte("orphaned-certificate-data"),
					"csr-name": []byte("non-existent-csr"),
				},
			}
			Expect(fakeClient.Create(ctx, orphanedSecret)).To(Succeed())
		})

		It("should remove the finalizer and delete the orphaned secret", func() {
			err := reconciler.cleanupOrphanedSecrets(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Check that the orphaned secret was deleted
			secretName := types.NamespacedName{
				Name:      "np-approval-test-namespace-orphaned",
				Namespace: namespace,
			}
			secret := &corev1.Secret{}
			err = fakeClient.Get(ctx, secretName, secret)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())
		})
	})
})
