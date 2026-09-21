package deploykit

import (
	"reflect"
	"strings"
	"testing"
)

func TestNewConfigDefaults(t *testing.T) {
	cfg := NewConfig()
	if cfg.AppName != "ledgerrag" {
		t.Errorf("AppName = %q, want ledgerrag", cfg.AppName)
	}
	if cfg.Image != "ghcr.io/example/ledgerrag:latest" {
		t.Errorf("Image = %q", cfg.Image)
	}
	if cfg.Replicas != 2 {
		t.Errorf("Replicas = %d, want 2", cfg.Replicas)
	}
	if !cfg.TLS {
		t.Error("TLS should default true")
	}
	if cfg.SecretBackend != "auto" {
		t.Errorf("SecretBackend = %q, want auto", cfg.SecretBackend)
	}
	if !cfg.DenyAllEgress {
		t.Error("DenyAllEgress should default true")
	}
}

func TestAllTargetsProducePlans(t *testing.T) {
	cfg := NewConfig()
	for _, name := range []string{"local", "azure", "aws"} {
		target, err := GetTarget(name)
		if err != nil {
			t.Fatalf("GetTarget(%q) error: %v", name, err)
		}
		plan := target.Plan(cfg)
		if len(plan.Steps) == 0 {
			t.Errorf("%s: expected non-empty steps", name)
		}
		if len(plan.Manifests) == 0 {
			t.Errorf("%s: expected manifests", name)
		}
		if !plan.IsSecure() {
			t.Errorf("%s: defaults should be secure", name)
		}
		if plan.Target != name {
			t.Errorf("%s: plan.Target = %q", name, plan.Target)
		}
	}
}

func TestSecureDefaultsPass(t *testing.T) {
	target, _ := GetTarget("azure")
	plan := target.Plan(NewConfig())
	if !plan.IsSecure() {
		t.Error("expected secure plan")
	}
	if len(plan.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", plan.Warnings)
	}
}

func TestInsecureConfigFlagged(t *testing.T) {
	cfg := NewConfig()
	cfg.TLS = false
	cfg.DenyAllEgress = false
	target, _ := GetTarget("aws")
	plan := target.Plan(cfg)
	if plan.IsSecure() {
		t.Error("expected insecure plan")
	}
	joined := strings.Join(plan.Warnings, "\n")
	if !strings.Contains(joined, "TLS") {
		t.Errorf("expected TLS warning, got %v", plan.Warnings)
	}
	if !strings.Contains(joined, "egress") {
		t.Errorf("expected egress warning, got %v", plan.Warnings)
	}
}

func TestPlaintextSecretBackendFlagged(t *testing.T) {
	cfg := NewConfig()
	cfg.SecretBackend = "plaintext"
	target, _ := GetTarget("local")
	plan := target.Plan(cfg)
	if plan.IsSecure() {
		t.Error("plaintext backend should be insecure")
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "plaintext") {
		t.Errorf("expected plaintext warning, got %v", plan.Warnings)
	}
}

func TestWarningOrdering(t *testing.T) {
	cfg := NewConfig()
	cfg.TLS = false
	cfg.DenyAllEgress = false
	cfg.SecretBackend = "plaintext"
	target, _ := GetTarget("local")
	got := target.Plan(cfg).Warnings
	want := []string{
		"TLS disabled -- traffic would be unencrypted",
		"egress not restricted -- data exfil risk",
		"secrets stored in plaintext",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("warnings = %v, want %v", got, want)
	}
}

func TestTargetSelectsRightSecretBackend(t *testing.T) {
	azure, _ := GetTarget("azure")
	if !strings.Contains(azure.Plan(NewConfig()).Manifests["main.bicep"], "keyvault") {
		t.Error("azure should use keyvault")
	}
	aws, _ := GetTarget("aws")
	if !strings.Contains(strings.Join(aws.Plan(NewConfig()).Steps, "\n"), "secrets-manager") {
		t.Error("aws should use secrets-manager")
	}
	local, _ := GetTarget("local")
	if !strings.Contains(local.Plan(NewConfig()).Manifests["values.yaml"], "k8s-secret") {
		t.Error("local should use k8s-secret")
	}
}

func TestNonAutoSecretBackendOverride(t *testing.T) {
	cfg := NewConfig()
	cfg.SecretBackend = "vault"
	local, _ := GetTarget("local")
	if !strings.Contains(local.Plan(cfg).Manifests["values.yaml"], "secretBackend: vault") {
		t.Error("override should propagate to manifest")
	}
}

func TestUnknownTargetError(t *testing.T) {
	_, err := GetTarget("gcp")
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	want := "unknown target 'gcp'. available: ['local', 'azure', 'aws']"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestHelmValuesExact(t *testing.T) {
	local, _ := GetTarget("local")
	got := local.Plan(NewConfig()).Manifests["values.yaml"]
	want := "image: ghcr.io/example/ledgerrag:latest\nreplicas: 2\ntls: true\nsecretBackend: k8s-secret\nnetworkPolicy:\n  denyAllEgress: true\n"
	if got != want {
		t.Errorf("values.yaml =\n%q\nwant\n%q", got, want)
	}
}

func TestBicepExact(t *testing.T) {
	azure, _ := GetTarget("azure")
	got := azure.Plan(NewConfig()).Manifests["main.bicep"]
	want := "// Bicep for ledgerrag\nparam image string = 'ghcr.io/example/ledgerrag:latest'\nparam secretBackend string = 'keyvault'\n"
	if got != want {
		t.Errorf("main.bicep =\n%q\nwant\n%q", got, want)
	}
}

func TestTerraformExact(t *testing.T) {
	aws, _ := GetTarget("aws")
	got := aws.Plan(NewConfig()).Manifests["main.tf"]
	want := "# Terraform for ledgerrag\nvariable \"image\" { default = \"ghcr.io/example/ledgerrag:latest\" }\n# secrets: secrets-manager\n"
	if got != want {
		t.Errorf("main.tf =\n%q\nwant\n%q", got, want)
	}
}

func TestTLSOffChangesIngressAndFrontSteps(t *testing.T) {
	cfg := NewConfig()
	cfg.TLS = false
	azure, _ := GetTarget("azure")
	azureSteps := strings.Join(azure.Plan(cfg).Steps, "\n")
	if strings.Contains(azureSteps, "with TLS") {
		t.Error("azure ingress step should drop TLS")
	}
	if !strings.Contains(azureSteps, "enable ingress") {
		t.Error("azure should still enable ingress")
	}
	aws, _ := GetTarget("aws")
	awsSteps := strings.Join(aws.Plan(cfg).Steps, "\n")
	if strings.Contains(awsSteps, "ACM TLS cert") {
		t.Error("aws should drop ACM TLS cert")
	}
	if !strings.Contains(awsSteps, "front with ALB") {
		t.Error("aws should still front with ALB")
	}
}

func TestDenyEgressOffRemovesStep(t *testing.T) {
	cfg := NewConfig()
	cfg.DenyAllEgress = false
	local, _ := GetTarget("local")
	steps := local.Plan(cfg).Steps
	if len(steps) != 4 {
		t.Errorf("expected 4 steps without egress lock, got %d", len(steps))
	}
	for _, s := range steps {
		if strings.Contains(s, "NetworkPolicy") {
			t.Error("egress-off plan should not add NetworkPolicy step")
		}
	}
}

func TestLocalStepsExactWithDefaults(t *testing.T) {
	local, _ := GetTarget("local")
	got := local.Plan(NewConfig()).Steps
	want := []string{
		"ensure kind/minikube cluster is running",
		"create namespace ledgerrag",
		"create k8s-secret for app credentials",
		"helm install ledgerrag ./charts/ledgerrag --set image=ghcr.io/example/ledgerrag:latest",
		"apply default-deny NetworkPolicy",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("steps = %v, want %v", got, want)
	}
}

func TestRenderPlanSecureReturnsZero(t *testing.T) {
	text, code, err := RenderPlan("local", NewConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(text, "secure-by-default checks: PASS") {
		t.Error("expected PASS line")
	}
	if !strings.HasPrefix(text, "== DeployKit plan: local / ledgerrag ==\n") {
		t.Errorf("unexpected header: %q", text)
	}
}

func TestRenderPlanInsecureReturnsOne(t *testing.T) {
	cfg := NewConfig()
	cfg.TLS = false
	cfg.DenyAllEgress = false
	text, code, err := RenderPlan("aws", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(text, "SECURITY WARNINGS:") {
		t.Error("expected warnings section")
	}
}

func TestRenderPlanUnknownTargetErrors(t *testing.T) {
	_, _, err := RenderPlan("gcp", NewConfig())
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
}

func TestTargetNamesOrder(t *testing.T) {
	got := TargetNames()
	want := []string{"local", "azure", "aws"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TargetNames = %v, want %v", got, want)
	}
}
