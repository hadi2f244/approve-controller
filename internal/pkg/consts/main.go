package consts

const SharedFinalizer = "hadiazad.local/finalizer"

// Definitions to manage status conditions
const (
	TypeAvailable = "Available"
	TypeError     = "Error"
	TypeDegraded  = "Degraded"
)

// Definitions to manage reasons conditions
const (
	//
	Reconciling = "Reconciling"
	//
	Finalizing = "Finalizing"
	//
	NetworkPolicyCreating = "NetworkPolicyCreating"
	//

)

var (
	CalicoOwnerKey = ".metadata.ownerReferences"
	ApiGVStr       = "hadiazad.local/v1alpha1"
)
