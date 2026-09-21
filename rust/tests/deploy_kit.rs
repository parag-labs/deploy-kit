use deploy_kit::{get_target, render_plan, Config, Target, TARGET_NAMES};

fn cfg() -> Config {
    Config::default()
}

#[test]
fn default_config_matches_python() {
    let c = Config::default();
    assert_eq!(c.app_name, "ledgerrag");
    assert_eq!(c.image, "ghcr.io/example/ledgerrag:latest");
    assert_eq!(c.replicas, 2);
    assert!(c.tls);
    assert_eq!(c.secret_backend, "auto");
    assert!(c.deny_all_egress);
}

#[test]
fn all_targets_produce_plans() {
    for name in ["local", "azure", "aws"] {
        let target = get_target(name).unwrap();
        let plan = target.plan(&cfg());
        assert!(!plan.steps.is_empty());
        assert!(!plan.manifests.is_empty());
        assert!(plan.is_secure());
        assert_eq!(plan.target, name);
    }
}

#[test]
fn secure_defaults_pass() {
    let plan = Target::Azure.plan(&cfg());
    assert!(plan.is_secure());
    assert!(plan.warnings.is_empty());
}

#[test]
fn insecure_config_flagged() {
    let mut c = cfg();
    c.tls = false;
    c.deny_all_egress = false;
    let plan = Target::Aws.plan(&c);
    assert!(!plan.is_secure());
    assert!(plan.warnings.iter().any(|w| w.contains("TLS")));
    assert!(plan.warnings.iter().any(|w| w.contains("egress")));
}

#[test]
fn plaintext_secret_backend_flagged() {
    let mut c = cfg();
    c.secret_backend = "plaintext".to_string();
    let plan = Target::Local.plan(&c);
    assert!(!plan.is_secure());
    assert_eq!(plan.warnings.len(), 1);
    assert!(plan.warnings[0].contains("plaintext"));
}

#[test]
fn warning_ordering_is_stable() {
    let mut c = cfg();
    c.tls = false;
    c.deny_all_egress = false;
    c.secret_backend = "plaintext".to_string();
    let plan = Target::Local.plan(&c);
    assert_eq!(
        plan.warnings,
        vec![
            "TLS disabled -- traffic would be unencrypted".to_string(),
            "egress not restricted -- data exfil risk".to_string(),
            "secrets stored in plaintext".to_string(),
        ]
    );
}

#[test]
fn target_selects_right_secret_backend() {
    assert!(Target::Azure
        .plan(&cfg())
        .manifest("main.bicep")
        .unwrap()
        .contains("keyvault"));
    assert!(Target::Aws
        .plan(&cfg())
        .steps
        .join("\n")
        .contains("secrets-manager"));
    assert!(Target::Local
        .plan(&cfg())
        .manifest("values.yaml")
        .unwrap()
        .contains("k8s-secret"));
}

#[test]
fn non_auto_secret_backend_override() {
    let mut c = cfg();
    c.secret_backend = "vault".to_string();
    let plan = Target::Local.plan(&c);
    assert!(plan
        .manifest("values.yaml")
        .unwrap()
        .contains("secretBackend: vault"));
}

#[test]
fn unknown_target_error_message() {
    let err = get_target("gcp").unwrap_err();
    assert_eq!(
        err,
        "unknown target 'gcp'. available: ['local', 'azure', 'aws']"
    );
}

#[test]
fn helm_values_exact() {
    let plan = Target::Local.plan(&cfg());
    assert_eq!(
        plan.manifest("values.yaml").unwrap(),
        "image: ghcr.io/example/ledgerrag:latest\nreplicas: 2\ntls: true\nsecretBackend: k8s-secret\nnetworkPolicy:\n  denyAllEgress: true\n"
    );
}

#[test]
fn bicep_exact() {
    let plan = Target::Azure.plan(&cfg());
    assert_eq!(
        plan.manifest("main.bicep").unwrap(),
        "// Bicep for ledgerrag\nparam image string = 'ghcr.io/example/ledgerrag:latest'\nparam secretBackend string = 'keyvault'\n"
    );
}

#[test]
fn terraform_exact() {
    let plan = Target::Aws.plan(&cfg());
    assert_eq!(
        plan.manifest("main.tf").unwrap(),
        "# Terraform for ledgerrag\nvariable \"image\" { default = \"ghcr.io/example/ledgerrag:latest\" }\n# secrets: secrets-manager\n"
    );
}

#[test]
fn tls_off_changes_ingress_and_front_steps() {
    let mut c = cfg();
    c.tls = false;
    let azure_steps = Target::Azure.plan(&c).steps.join("\n");
    assert!(!azure_steps.contains("with TLS"));
    assert!(azure_steps.contains("enable ingress"));
    let aws_steps = Target::Aws.plan(&c).steps.join("\n");
    assert!(!aws_steps.contains("ACM TLS cert"));
    assert!(aws_steps.contains("front with ALB"));
}

#[test]
fn deny_egress_off_removes_step() {
    let mut c = cfg();
    c.deny_all_egress = false;
    let steps = Target::Local.plan(&c).steps;
    assert_eq!(steps.len(), 4);
    assert!(!steps.iter().any(|s| s.contains("NetworkPolicy")));
}

#[test]
fn local_steps_exact_with_defaults() {
    let steps = Target::Local.plan(&cfg()).steps;
    assert_eq!(
        steps,
        vec![
            "ensure kind/minikube cluster is running".to_string(),
            "create namespace ledgerrag".to_string(),
            "create k8s-secret for app credentials".to_string(),
            "helm install ledgerrag ./charts/ledgerrag --set image=ghcr.io/example/ledgerrag:latest"
                .to_string(),
            "apply default-deny NetworkPolicy".to_string(),
        ]
    );
}

#[test]
fn render_plan_secure_returns_zero() {
    let (text, code) = render_plan("local", &cfg()).unwrap();
    assert_eq!(code, 0);
    assert!(text.contains("secure-by-default checks: PASS"));
    assert!(text.starts_with("== DeployKit plan: local / ledgerrag ==\n"));
}

#[test]
fn render_plan_insecure_returns_one() {
    let mut c = cfg();
    c.tls = false;
    c.deny_all_egress = false;
    let (text, code) = render_plan("aws", &c).unwrap();
    assert_eq!(code, 1);
    assert!(text.contains("SECURITY WARNINGS:"));
}

#[test]
fn render_plan_unknown_target_errors() {
    assert!(render_plan("gcp", &cfg()).is_err());
}

#[test]
fn target_names_order() {
    assert_eq!(TARGET_NAMES, ["local", "azure", "aws"]);
}

#[test]
fn target_name_method() {
    assert_eq!(Target::Local.name(), "local");
    assert_eq!(Target::Azure.name(), "azure");
    assert_eq!(Target::Aws.name(), "aws");
}
