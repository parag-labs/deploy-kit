"""DeployKit tests: plan generation + secure-by-default enforcement across targets."""

import pytest

from deploykit.cli import main
from deploykit.targets import DeployConfig, get_target


def test_all_targets_produce_plans():
    cfg = DeployConfig()
    for name in ("local", "azure", "aws"):
        plan = get_target(name).plan(cfg)
        assert plan.steps
        assert plan.manifests
        assert plan.is_secure() is True  # defaults are secure


def test_secure_defaults_pass():
    plan = get_target("azure").plan(DeployConfig())
    assert plan.is_secure()
    assert not plan.warnings


def test_insecure_config_flagged():
    plan = get_target("aws").plan(DeployConfig(tls=False, deny_all_egress=False))
    assert not plan.is_secure()
    assert any("TLS" in w for w in plan.warnings)
    assert any("egress" in w for w in plan.warnings)


def test_target_selects_right_secret_backend():
    assert "keyvault" in get_target("azure").plan(DeployConfig()).manifests["main.bicep"]
    assert "secrets-manager" in "\n".join(get_target("aws").plan(DeployConfig()).steps)


def test_unknown_target_raises():
    with pytest.raises(ValueError):
        get_target("gcp")


def test_cli_plan_secure_returns_zero():
    assert main(["plan", "--target", "local"]) == 0


def test_cli_plan_insecure_returns_nonzero():
    assert main(["plan", "--target", "aws", "--no-tls", "--allow-egress"]) == 1
