# Approve Controller Development Guidelines

This document provides essential information for developers working on the Approve Controller project.

## Build and Configuration Instructions

### Prerequisites
- Go version v1.23.0+
- Docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster

### Local Development Setup

1. **Clone the repository**:
   ```bash
   git clone <repository-url>
   cd approve-controller
   ```

2. **Build the project**:
   ```bash
   make build
   ```
   This will compile the controller binary to `bin/manager`.

3. **Run locally**:
   ```bash
   make run
   ```
   This command:
   - Generates manifests
   - Installs temporary configuration
   - Sets up local webhook configuration
   - Updates /etc/hosts with the webhook domain
   - Retrieves certificates
   - Runs the controller locally

4. **Build and push Docker image**:
   ```bash
   make docker-build docker-push IMG=<registry>/approve-controller:tag
   ```

5. **Deploy to Kubernetes**:
   ```bash
   make deploy IMG=<registry>/approve-controller:tag
   ```

### Configuration

The controller uses a configuration file located at `/tmp/config.yaml` when running locally. This file is generated from the ConfigMap in `config/manager`.

## Testing Information

### Test Structure

The project uses the following testing frameworks:
- **Ginkgo**: BDD-style testing framework
- **Gomega**: Matcher/assertion library
- **controller-runtime/envtest**: For setting up a test Kubernetes API server

Tests are organized into:
- **Unit tests**: Located in the same package as the code they test
- **End-to-end tests**: Located in the `test/e2e` directory

### Running Tests

1. **Run all unit tests**:
   ```bash
   make test
   ```
   This command:
   - Generates manifests
   - Formats code
   - Runs `go vet`
   - Sets up the test environment
   - Runs all tests except e2e tests
   - Generates a coverage report in `cover.out`

2. **Run specific tests**:
   ```bash
   go test ./path/to/package -v
   ```

3. **Run e2e tests**:
   ```bash
   make test-e2e
   ```
   Note: This requires a running Kind cluster.

### Adding New Tests

#### Unit Tests

1. Create a test file in the same package as the code you're testing, with the naming convention `<filename>_test.go`.

2. For controller tests, use the Ginkgo BDD framework:
   ```
   var _ = Describe("Your Component", func() {
       Context("When doing something specific", func() {
           It("should behave in a certain way", func() {
               // Test code here
           })
       })
   })
   ```

3. For simpler components, use standard Go testing:
   ```
   func TestYourFunction(t *testing.T) {
       // Test setup
       result := YourFunction()
       // Assertions
       if result != expectedResult {
           t.Errorf("Expected %v, got %v", expectedResult, result)
       }
   }
   ```

#### Example: Testing a Hash Function

Here's a simple test for the `generateNetworkPolicyHash` function:

```
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
```

Run this test with:
```bash
cd internal/webhook/v1 && go test -v -run TestGenerateNetworkPolicyHash
```

## Additional Development Information

### Project Structure

- **cmd/**: Contains the main application entry point
- **internal/**: Contains the internal code
  - **controller/**: Contains the Kubernetes controllers
  - **webhook/**: Contains the admission webhooks
  - **pkg/**: Contains shared packages
- **config/**: Contains Kubernetes manifests
  - **crd/**: Custom Resource Definitions
  - **rbac/**: RBAC configurations
  - **manager/**: Controller manager configuration
  - **webhook/**: Webhook configuration
- **test/**: Contains test code
  - **e2e/**: End-to-end tests
  - **utils/**: Test utilities

### Code Style

The project follows standard Go code style conventions:
- Use `gofmt` for code formatting
- Follow [Effective Go](https://golang.org/doc/effective_go) guidelines
- Use linting tools to ensure code quality:
  ```bash
  make lint
  ```

### Webhook Development

The project uses Kubernetes admission webhooks:
- **Validating Webhooks**: Validate resources before they are created or updated
- **Mutating Webhooks**: Modify resources before they are created or updated

When developing webhooks:
1. Define the webhook in the source code with appropriate annotations
2. Generate the webhook configuration with `make manifests`
3. Test the webhook locally with `make run`

### Debugging

For local debugging:
1. Run the controller with verbose logging:
   ```bash
   OPERATOR_CONFIG_PATH=/tmp/config.yaml go run ./cmd/main.go --webhook-cert-path=/tmp/k8s-webhook-server/serving-certs --webhook-cert-name=tls.crt --webhook-cert-key=tls.key -v=4
   ```

2. Use the Kubernetes API server logs to debug webhook issues:
   ```bash
   kubectl logs -n kube-system -l component=kube-apiserver
   ```

3. Check webhook configuration:
   ```bash
   kubectl get validatingwebhookconfigurations
   kubectl get mutatingwebhookconfigurations
   ```

### Common Issues

1. **Webhook Certificate Issues**: Ensure certificates are properly configured and the webhook service is reachable
2. **RBAC Issues**: Check that the controller has the necessary permissions
3. **CRD Issues**: Ensure CRDs are properly installed and registered
