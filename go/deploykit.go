// Package deploykit renders deterministic, secure-by-default deployment plans
// for a reference LLM app across the local, azure, and aws targets.
//
// It is a faithful port of the Python deploykit core: configuration defaults,
// plan and manifest generation, target selection, and the secure-by-default
// checks. It deliberately excludes the argparse CLI wiring and any real cloud
// apply/destroy calls, which are stubs in the Python project as well.
package deploykit

import (
	"fmt"
	"strings"
)

// Config holds the deployment configuration. Use NewConfig to obtain the
// secure-by-default values that match the Python DeployConfig dataclass.
type Config struct {
	AppName       string
	Image         string
	Replicas      int
	TLS           bool
	SecretBackend string
	DenyAllEgress bool
}

// NewConfig returns a Config populated with the same secure defaults as the
// Python DeployConfig dataclass.
func NewConfig() Config {
	return Config{
		AppName:       "ledgerrag",
		Image:         "ghcr.io/example/ledgerrag:latest",
		Replicas:      2,
		TLS:           true,
		SecretBackend: "auto",
		DenyAllEgress: true,
	}
}

// Plan is the rendered deployment plan for a single target.
type Plan struct {
	Target    string
	Steps     []string
	Manifests map[string]string
	Warnings  []string
}

// IsSecure reports whether the plan tripped no secure-by-default warnings.
func (p Plan) IsSecure() bool {
	return len(p.Warnings) == 0
}

// securityChecks returns the secure-by-default warnings for cfg, in the same
// order the Python implementation appends them.
func securityChecks(cfg Config) []string {
	warnings := []string{}
	if !cfg.TLS {
		warnings = append(warnings, "TLS disabled -- traffic would be unencrypted")
	}
	if !cfg.DenyAllEgress {
		warnings = append(warnings, "egress not restricted -- data exfil risk")
	}
	if cfg.SecretBackend == "plaintext" {
		warnings = append(warnings, "secrets stored in plaintext")
	}
	return warnings
}

// Target renders a deployment Plan for a specific environment.
type Target interface {
	// Name returns the target's canonical name.
	Name() string
	// Plan renders the deployment plan for cfg.
	Plan(cfg Config) Plan
}

type localTarget struct{}

func (localTarget) Name() string { return "local" }

func (localTarget) Plan(cfg Config) Plan {
	secretBackend := "k8s-secret"
	if cfg.SecretBackend != "auto" {
		secretBackend = cfg.SecretBackend
	}
	steps := []string{
		"ensure kind/minikube cluster is running",
		fmt.Sprintf("create namespace %s", cfg.AppName),
		fmt.Sprintf("create %s for app credentials", secretBackend),
		fmt.Sprintf("helm install %s ./charts/%s --set image=%s", cfg.AppName, cfg.AppName, cfg.Image),
	}
	if cfg.DenyAllEgress {
		steps = append(steps, "apply default-deny NetworkPolicy")
	}
	return Plan{
		Target:    "local",
		Steps:     steps,
		Manifests: map[string]string{"values.yaml": helmValues(cfg, secretBackend)},
		Warnings:  securityChecks(cfg),
	}
}

type azureTarget struct{}

func (azureTarget) Name() string { return "azure" }

func (azureTarget) Plan(cfg Config) Plan {
	secretBackend := "keyvault"
	if cfg.SecretBackend != "auto" {
		secretBackend = cfg.SecretBackend
	}
	ingress := "enable ingress"
	if cfg.TLS {
		ingress = "bind managed identity; enable ingress with TLS"
	}
	steps := []string{
		"az group create + provision Azure Container Apps environment (Bicep)",
		fmt.Sprintf("store secrets in Azure %s", secretBackend),
		fmt.Sprintf("deploy %s container app (image=%s, replicas=%d)", cfg.AppName, cfg.Image, cfg.Replicas),
		ingress,
	}
	if cfg.DenyAllEgress {
		steps = append(steps, "apply egress restrictions via NSG / container app policy")
	}
	return Plan{
		Target:    "azure",
		Steps:     steps,
		Manifests: map[string]string{"main.bicep": bicep(cfg, secretBackend)},
		Warnings:  securityChecks(cfg),
	}
}

type awsTarget struct{}

func (awsTarget) Name() string { return "aws" }

func (awsTarget) Plan(cfg Config) Plan {
	secretBackend := "secrets-manager"
	if cfg.SecretBackend != "auto" {
		secretBackend = cfg.SecretBackend
	}
	front := "front with ALB"
	if cfg.TLS {
		front = "front with ALB + ACM TLS cert"
	}
	steps := []string{
		"terraform apply: VPC + ECS/Fargate cluster",
		fmt.Sprintf("store secrets in AWS %s", secretBackend),
		fmt.Sprintf("deploy %s service (image=%s, desired_count=%d)", cfg.AppName, cfg.Image, cfg.Replicas),
		front,
	}
	if cfg.DenyAllEgress {
		steps = append(steps, "apply restrictive security groups (deny-all egress baseline)")
	}
	return Plan{
		Target:    "aws",
		Steps:     steps,
		Manifests: map[string]string{"main.tf": terraform(cfg, secretBackend)},
		Warnings:  securityChecks(cfg),
	}
}

// targetNames lists the registered targets in canonical order, matching the
// insertion order of Python's TARGETS dict.
var targetNames = []string{"local", "azure", "aws"}

var targets = map[string]Target{
	"local": localTarget{},
	"azure": azureTarget{},
	"aws":   awsTarget{},
}

// GetTarget returns the target registered under name, or an error whose message
// matches the Python ValueError ("unknown target '<name>'. available: [...]").
func GetTarget(name string) (Target, error) {
	t, ok := targets[name]
	if !ok {
		return nil, fmt.Errorf("unknown target '%s'. available: %s", name, formatNameList(targetNames))
	}
	return t, nil
}

// TargetNames returns the canonical target names in order.
func TargetNames() []string {
	out := make([]string, len(targetNames))
	copy(out, targetNames)
	return out
}

// RenderPlan renders the human-readable plan for targetName using cfg and
// returns the text alongside the intended process exit code (1 when
// secure-by-default warnings fire, otherwise 0). It mirrors the output of the
// Python CLI's _print_plan.
func RenderPlan(targetName string, cfg Config) (string, int, error) {
	target, err := GetTarget(targetName)
	if err != nil {
		return "", 0, err
	}
	plan := target.Plan(cfg)
	var b strings.Builder
	fmt.Fprintf(&b, "== DeployKit plan: %s / %s ==\n", plan.Target, cfg.AppName)
	for i, step := range plan.Steps {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, step)
	}
	if len(plan.Warnings) > 0 {
		b.WriteString("\n  SECURITY WARNINGS:\n")
		for _, w := range plan.Warnings {
			fmt.Fprintf(&b, "    ! %s\n", w)
		}
		return b.String(), 1, nil
	}
	b.WriteString("\n  secure-by-default checks: PASS\n")
	return b.String(), 0, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func helmValues(cfg Config, secretBackend string) string {
	return fmt.Sprintf(
		"image: %s\nreplicas: %d\ntls: %s\nsecretBackend: %s\nnetworkPolicy:\n  denyAllEgress: %s\n",
		cfg.Image, cfg.Replicas, boolStr(cfg.TLS), secretBackend, boolStr(cfg.DenyAllEgress),
	)
}

func bicep(cfg Config, secretBackend string) string {
	return fmt.Sprintf(
		"// Bicep for %s\nparam image string = '%s'\nparam secretBackend string = '%s'\n",
		cfg.AppName, cfg.Image, secretBackend,
	)
}

func terraform(cfg Config, secretBackend string) string {
	return fmt.Sprintf(
		"# Terraform for %s\nvariable \"image\" { default = \"%s\" }\n# secrets: %s\n",
		cfg.AppName, cfg.Image, secretBackend,
	)
}

func formatNameList(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "'" + n + "'"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
