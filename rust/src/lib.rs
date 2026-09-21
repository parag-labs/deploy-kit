//! Deterministic, secure-by-default deployment plans for a reference LLM app
//! across the `local`, `azure`, and `aws` targets.
//!
//! This is a faithful port of the Python `deploykit` core: configuration
//! defaults, plan and manifest generation, target selection, and the
//! secure-by-default checks. It deliberately excludes the argparse CLI wiring
//! and any real cloud apply/destroy calls, which are stubs in the Python
//! project as well.

use std::collections::HashMap;

/// Deployment configuration, mirroring the Python `DeployConfig` dataclass.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Config {
    pub app_name: String,
    pub image: String,
    pub replicas: i64,
    pub tls: bool,
    pub secret_backend: String,
    pub deny_all_egress: bool,
}

impl Default for Config {
    /// Returns the same secure defaults as the Python `DeployConfig`.
    fn default() -> Self {
        Config {
            app_name: "ledgerrag".to_string(),
            image: "ghcr.io/example/ledgerrag:latest".to_string(),
            replicas: 2,
            tls: true,
            secret_backend: "auto".to_string(),
            deny_all_egress: true,
        }
    }
}

/// The rendered deployment plan for a single target.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Plan {
    pub target: String,
    pub steps: Vec<String>,
    pub manifests: HashMap<String, String>,
    pub warnings: Vec<String>,
}

impl Plan {
    /// Reports whether the plan tripped no secure-by-default warnings.
    pub fn is_secure(&self) -> bool {
        self.warnings.is_empty()
    }

    /// Returns the manifest body registered under `name`, if any.
    pub fn manifest(&self, name: &str) -> Option<&str> {
        self.manifests.get(name).map(String::as_str)
    }
}

/// The canonical target names, in the same order as the Python `TARGETS` dict.
pub const TARGET_NAMES: [&str; 3] = ["local", "azure", "aws"];

/// A deployment target. Each variant renders a [`Plan`] deterministically.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Target {
    Local,
    Azure,
    Aws,
}

impl Target {
    /// Returns the target's canonical name.
    pub fn name(&self) -> &'static str {
        match self {
            Target::Local => "local",
            Target::Azure => "azure",
            Target::Aws => "aws",
        }
    }

    /// Renders the deployment plan for `cfg`.
    pub fn plan(&self, cfg: &Config) -> Plan {
        match self {
            Target::Local => plan_local(cfg),
            Target::Azure => plan_azure(cfg),
            Target::Aws => plan_aws(cfg),
        }
    }
}

/// Resolves a target by name, returning an error whose message matches the
/// Python `ValueError` ("unknown target '<name>'. available: [...]").
pub fn get_target(name: &str) -> Result<Target, String> {
    match name {
        "local" => Ok(Target::Local),
        "azure" => Ok(Target::Azure),
        "aws" => Ok(Target::Aws),
        _ => Err(format!(
            "unknown target '{}'. available: {}",
            name,
            format_name_list(&TARGET_NAMES)
        )),
    }
}

/// Renders the human-readable plan text for `target_name` and returns it along
/// with the intended process exit code (1 when secure-by-default warnings fire,
/// otherwise 0). Mirrors the output of the Python CLI's `_print_plan`.
pub fn render_plan(target_name: &str, cfg: &Config) -> Result<(String, i32), String> {
    let target = get_target(target_name)?;
    let plan = target.plan(cfg);
    let mut out = String::new();
    out.push_str(&format!(
        "== DeployKit plan: {} / {} ==\n",
        plan.target, cfg.app_name
    ));
    for (i, step) in plan.steps.iter().enumerate() {
        out.push_str(&format!("  {}. {}\n", i + 1, step));
    }
    if !plan.warnings.is_empty() {
        out.push_str("\n  SECURITY WARNINGS:\n");
        for w in &plan.warnings {
            out.push_str(&format!("    ! {}\n", w));
        }
        return Ok((out, 1));
    }
    out.push_str("\n  secure-by-default checks: PASS\n");
    Ok((out, 0))
}

fn security_checks(cfg: &Config) -> Vec<String> {
    let mut warnings = Vec::new();
    if !cfg.tls {
        warnings.push("TLS disabled -- traffic would be unencrypted".to_string());
    }
    if !cfg.deny_all_egress {
        warnings.push("egress not restricted -- data exfil risk".to_string());
    }
    if cfg.secret_backend == "plaintext" {
        warnings.push("secrets stored in plaintext".to_string());
    }
    warnings
}

fn plan_local(cfg: &Config) -> Plan {
    let secret_backend: &str = if cfg.secret_backend == "auto" {
        "k8s-secret"
    } else {
        cfg.secret_backend.as_str()
    };
    let mut steps = vec![
        "ensure kind/minikube cluster is running".to_string(),
        format!("create namespace {}", cfg.app_name),
        format!("create {} for app credentials", secret_backend),
        format!(
            "helm install {} ./charts/{} --set image={}",
            cfg.app_name, cfg.app_name, cfg.image
        ),
    ];
    if cfg.deny_all_egress {
        steps.push("apply default-deny NetworkPolicy".to_string());
    }
    let mut manifests = HashMap::new();
    manifests.insert("values.yaml".to_string(), helm_values(cfg, secret_backend));
    Plan {
        target: "local".to_string(),
        steps,
        manifests,
        warnings: security_checks(cfg),
    }
}

fn plan_azure(cfg: &Config) -> Plan {
    let secret_backend: &str = if cfg.secret_backend == "auto" {
        "keyvault"
    } else {
        cfg.secret_backend.as_str()
    };
    let ingress = if cfg.tls {
        "bind managed identity; enable ingress with TLS"
    } else {
        "enable ingress"
    };
    let mut steps = vec![
        "az group create + provision Azure Container Apps environment (Bicep)".to_string(),
        format!("store secrets in Azure {}", secret_backend),
        format!(
            "deploy {} container app (image={}, replicas={})",
            cfg.app_name, cfg.image, cfg.replicas
        ),
        ingress.to_string(),
    ];
    if cfg.deny_all_egress {
        steps.push("apply egress restrictions via NSG / container app policy".to_string());
    }
    let mut manifests = HashMap::new();
    manifests.insert("main.bicep".to_string(), bicep(cfg, secret_backend));
    Plan {
        target: "azure".to_string(),
        steps,
        manifests,
        warnings: security_checks(cfg),
    }
}

fn plan_aws(cfg: &Config) -> Plan {
    let secret_backend: &str = if cfg.secret_backend == "auto" {
        "secrets-manager"
    } else {
        cfg.secret_backend.as_str()
    };
    let front = if cfg.tls {
        "front with ALB + ACM TLS cert"
    } else {
        "front with ALB"
    };
    let mut steps = vec![
        "terraform apply: VPC + ECS/Fargate cluster".to_string(),
        format!("store secrets in AWS {}", secret_backend),
        format!(
            "deploy {} service (image={}, desired_count={})",
            cfg.app_name, cfg.image, cfg.replicas
        ),
        front.to_string(),
    ];
    if cfg.deny_all_egress {
        steps.push("apply restrictive security groups (deny-all egress baseline)".to_string());
    }
    let mut manifests = HashMap::new();
    manifests.insert("main.tf".to_string(), terraform(cfg, secret_backend));
    Plan {
        target: "aws".to_string(),
        steps,
        manifests,
        warnings: security_checks(cfg),
    }
}

fn bool_str(b: bool) -> &'static str {
    if b {
        "true"
    } else {
        "false"
    }
}

fn helm_values(cfg: &Config, secret_backend: &str) -> String {
    format!(
        "image: {}\nreplicas: {}\ntls: {}\nsecretBackend: {}\nnetworkPolicy:\n  denyAllEgress: {}\n",
        cfg.image,
        cfg.replicas,
        bool_str(cfg.tls),
        secret_backend,
        bool_str(cfg.deny_all_egress)
    )
}

fn bicep(cfg: &Config, secret_backend: &str) -> String {
    format!(
        "// Bicep for {}\nparam image string = '{}'\nparam secretBackend string = '{}'\n",
        cfg.app_name, cfg.image, secret_backend
    )
}

fn terraform(cfg: &Config, secret_backend: &str) -> String {
    format!(
        "# Terraform for {}\nvariable \"image\" {{ default = \"{}\" }}\n# secrets: {}\n",
        cfg.app_name, cfg.image, secret_backend
    )
}

fn format_name_list(names: &[&str]) -> String {
    let quoted: Vec<String> = names.iter().map(|n| format!("'{}'", n)).collect();
    format!("[{}]", quoted.join(", "))
}
