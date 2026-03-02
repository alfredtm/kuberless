package shared

const (
	// LabelManagedBy identifies resources managed by this platform.
	LabelManagedBy = "app.kubernetes.io/managed-by"
	// LabelManagedByValue is the value for the managed-by label.
	LabelManagedByValue = "kuberless"

	// LabelTenant identifies the owning tenant.
	LabelTenant = "kuberless.io/tenant"
	// LabelApp identifies the owning app.
	LabelApp = "kuberless.io/app"

	// FinalizerTenant is the finalizer for tenant resources.
	FinalizerTenant = "kuberless.io/tenant-finalizer"
	// FinalizerApp is the finalizer for app resources.
	FinalizerApp = "kuberless.io/app-finalizer"

	// SystemNamespace is the namespace where the platform operator runs.
	SystemNamespace = "kuberless-system"
	// KnativeServingNamespace is the namespace where Knative Serving runs.
	KnativeServingNamespace = "knative-serving"

	// DefaultDomain is the default base domain for app URLs.
	DefaultDomain = "kuberless.example.com"
)

// PlanDefaults holds default resource limits for each billing plan.
var PlanDefaults = map[string]PlanResourceDefaults{
	"free": {
		MaxCPU:    "1",
		MaxMemory: "2Gi",
		MaxApps:   3,
	},
	"starter": {
		MaxCPU:    "4",
		MaxMemory: "8Gi",
		MaxApps:   10,
	},
	"pro": {
		MaxCPU:    "16",
		MaxMemory: "32Gi",
		MaxApps:   50,
	},
	"enterprise": {
		MaxCPU:    "64",
		MaxMemory: "128Gi",
		MaxApps:   200,
	},
}

// PlanResourceDefaults holds the default resource limits for a plan.
type PlanResourceDefaults struct {
	MaxCPU    string
	MaxMemory string
	MaxApps   int32
}

// TenantNamespaceName returns the namespace name for a tenant.
func TenantNamespaceName(tenantName string) string {
	return "tenant-" + tenantName
}
