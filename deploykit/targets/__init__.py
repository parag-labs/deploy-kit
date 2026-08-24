"""DeployKit deployment targets: pluggable providers with secure defaults.

Each target renders a deployment plan (secrets mgmt, TLS, network policy) for a
reference LLM app. This scaffold produces plans/manifests deterministically so it
is testable without real cloud credentials; wire real Terraform/Helm apply calls
behind the same interface for production.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Protocol


@dataclass
class DeployConfig:
    app_name: str = "ledgerrag"
    image: str = "ghcr.io/example/ledgerrag:latest"
    replicas: int = 2
    tls: bool = True
    secret_backend: str = "auto"      # auto-selected per target
    deny_all_egress: bool = True      # secure-by-default network policy


@dataclass
class DeployPlan:
    target: str
    steps: list[str] = field(default_factory=list)
    manifests: dict[str, str] = field(default_factory=dict)
    warnings: list[str] = field(default_factory=list)

    def is_secure(self) -> bool:
        return not self.warnings


class Target(Protocol):
    name: str
    def plan(self, cfg: DeployConfig) -> DeployPlan: ...


def _security_checks(cfg: DeployConfig) -> list[str]:
    warnings = []
    if not cfg.tls:
        warnings.append("TLS disabled -- traffic would be unencrypted")
    if not cfg.deny_all_egress:
        warnings.append("egress not restricted -- data exfil risk")
    if cfg.secret_backend == "plaintext":
        warnings.append("secrets stored in plaintext")
    return warnings


class LocalTarget:
    name = "local"

    def plan(self, cfg: DeployConfig) -> DeployPlan:
        secret_backend = "k8s-secret" if cfg.secret_backend == "auto" else cfg.secret_backend
        steps = [
            "ensure kind/minikube cluster is running",
            f"create namespace {cfg.app_name}",
            f"create {secret_backend} for app credentials",
            f"helm install {cfg.app_name} ./charts/{cfg.app_name} --set image={cfg.image}",
        ]
        if cfg.deny_all_egress:
            steps.append("apply default-deny NetworkPolicy")
        manifests = {"values.yaml": _helm_values(cfg, secret_backend)}
        return DeployPlan("local", steps, manifests, _security_checks(cfg))


class AzureTarget:
    name = "azure"

    def plan(self, cfg: DeployConfig) -> DeployPlan:
        secret_backend = "keyvault" if cfg.secret_backend == "auto" else cfg.secret_backend
        steps = [
            "az group create + provision Azure Container Apps environment (Bicep)",
            f"store secrets in Azure {secret_backend}",
            f"deploy {cfg.app_name} container app (image={cfg.image}, replicas={cfg.replicas})",
            "bind managed identity; enable ingress with TLS" if cfg.tls else "enable ingress",
        ]
        if cfg.deny_all_egress:
            steps.append("apply egress restrictions via NSG / container app policy")
        manifests = {"main.bicep": _bicep(cfg, secret_backend)}
        return DeployPlan("azure", steps, manifests, _security_checks(cfg))


class AwsTarget:
    name = "aws"

    def plan(self, cfg: DeployConfig) -> DeployPlan:
        secret_backend = "secrets-manager" if cfg.secret_backend == "auto" else cfg.secret_backend
        steps = [
            "terraform apply: VPC + ECS/Fargate cluster",
            f"store secrets in AWS {secret_backend}",
            f"deploy {cfg.app_name} service (image={cfg.image}, desired_count={cfg.replicas})",
            "front with ALB + ACM TLS cert" if cfg.tls else "front with ALB",
        ]
        if cfg.deny_all_egress:
            steps.append("apply restrictive security groups (deny-all egress baseline)")
        manifests = {"main.tf": _terraform(cfg, secret_backend)}
        return DeployPlan("aws", steps, manifests, _security_checks(cfg))


TARGETS: dict[str, Target] = {t.name: t for t in (LocalTarget(), AzureTarget(), AwsTarget())}


def get_target(name: str) -> Target:
    if name not in TARGETS:
        raise ValueError(f"unknown target '{name}'. available: {list(TARGETS)}")
    return TARGETS[name]


# --- tiny manifest renderers (illustrative) ---
def _helm_values(cfg: DeployConfig, secret_backend: str) -> str:
    return (
        f"image: {cfg.image}\nreplicas: {cfg.replicas}\n"
        f"tls: {str(cfg.tls).lower()}\nsecretBackend: {secret_backend}\n"
        f"networkPolicy:\n  denyAllEgress: {str(cfg.deny_all_egress).lower()}\n"
    )


def _bicep(cfg: DeployConfig, secret_backend: str) -> str:
    return f"// Bicep for {cfg.app_name}\nparam image string = '{cfg.image}'\nparam secretBackend string = '{secret_backend}'\n"


def _terraform(cfg: DeployConfig, secret_backend: str) -> str:
    return f'# Terraform for {cfg.app_name}\nvariable "image" {{ default = "{cfg.image}" }}\n# secrets: {secret_backend}\n'
