"""DeployKit CLI: one command to plan/deploy an LLM app into any environment.

    deploykit up --target azure --app ledgerrag
    deploykit plan --target local
    deploykit down --target aws --app ledgerrag

This scaffold prints the plan (with secure-by-default checks). Wire real
apply/destroy calls behind the same interface for production use.
"""

from __future__ import annotations

import argparse
import sys

from deploykit.targets import DeployConfig, get_target


def _build_config(args: argparse.Namespace) -> DeployConfig:
    return DeployConfig(
        app_name=args.app,
        image=args.image,
        replicas=args.replicas,
        tls=not args.no_tls,
        deny_all_egress=not args.allow_egress,
    )


def _print_plan(target_name: str, cfg: DeployConfig) -> int:
    target = get_target(target_name)
    plan = target.plan(cfg)
    print(f"== DeployKit plan: {plan.target} / {cfg.app_name} ==")
    for i, step in enumerate(plan.steps, 1):
        print(f"  {i}. {step}")
    if plan.warnings:
        print("\n  SECURITY WARNINGS:")
        for w in plan.warnings:
            print(f"    ! {w}")
        return 1
    print("\n  secure-by-default checks: PASS")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="deploykit")
    parser.add_argument("command", choices=["up", "plan", "down", "status"])
    parser.add_argument("--target", default="local", help="local | azure | aws")
    parser.add_argument("--app", default="ledgerrag")
    parser.add_argument("--image", default="ghcr.io/example/ledgerrag:latest")
    parser.add_argument("--replicas", type=int, default=2)
    parser.add_argument("--no-tls", action="store_true")
    parser.add_argument("--allow-egress", action="store_true")
    args = parser.parse_args(argv)

    cfg = _build_config(args)

    if args.command in ("up", "plan"):
        return _print_plan(args.target, cfg)
    if args.command == "down":
        print(f"== DeployKit teardown: {args.target} / {cfg.app_name} ==")
        print("  destroying resources (idempotent)...")
        return 0
    print(f"status: {args.target}/{cfg.app_name} -> (scaffold) not tracked")
    return 0


if __name__ == "__main__":
    sys.exit(main())
