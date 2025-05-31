package v1

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateNetworkPolicyHash(t *testing.T) {
	// Create a simple NetworkPolicy for testing
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
		},
	}

	// Generate hash
	hash, err := generateNetworkPolicyHash(np)
	if err != nil {
		t.Fatalf("Failed to generate hash: %v", err)
	}

	// Verify hash is not empty
	if hash == "" {
		t.Error("Generated hash is empty")
	}

	// Generate hash again to verify consistency
	hash2, err := generateNetworkPolicyHash(np)
	if err != nil {
		t.Fatalf("Failed to generate hash second time: %v", err)
	}

	// Verify hashes are the same
	if hash != hash2 {
		t.Errorf("Hashes are not consistent: %s != %s", hash, hash2)
	}

	// Modify the NetworkPolicy
	np.Name = "modified-policy"

	// Generate hash for modified policy
	modifiedHash, err := generateNetworkPolicyHash(np)
	if err != nil {
		t.Fatalf("Failed to generate hash for modified policy: %v", err)
	}

	// Verify the hash changed
	if hash == modifiedHash {
		t.Error("Hash did not change after modifying the NetworkPolicy")
	}
}
