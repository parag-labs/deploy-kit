import { describe, expect, it } from "vitest";
import {
  type Config,
  TARGET_NAMES,
  defaultConfig,
  getTarget,
  isSecure,
  renderPlan,
} from "./deploykit.js";

function cfg(): Config {
  return defaultConfig();
}

describe("deploykit core", () => {
  it("default config matches python", () => {
    const c = defaultConfig();
    expect(c.appName).toBe("ledgerrag");
    expect(c.image).toBe("ghcr.io/example/ledgerrag:latest");
    expect(c.replicas).toBe(2);
    expect(c.tls).toBe(true);
    expect(c.secretBackend).toBe("auto");
    expect(c.denyAllEgress).toBe(true);
  });

  it.each(["local", "azure", "aws"])("target %s produces a plan", (name) => {
    const plan = getTarget(name).plan(cfg());
    expect(plan.steps.length).toBeGreaterThan(0);
    expect(Object.keys(plan.manifests).length).toBeGreaterThan(0);
    expect(isSecure(plan)).toBe(true);
    expect(plan.target).toBe(name);
  });

  it("secure defaults pass", () => {
    const plan = getTarget("azure").plan(cfg());
    expect(isSecure(plan)).toBe(true);
    expect(plan.warnings).toHaveLength(0);
  });

  it("insecure config is flagged", () => {
    const c = cfg();
    c.tls = false;
    c.denyAllEgress = false;
    const plan = getTarget("aws").plan(c);
    expect(isSecure(plan)).toBe(false);
    expect(plan.warnings.some((w) => w.includes("TLS"))).toBe(true);
    expect(plan.warnings.some((w) => w.includes("egress"))).toBe(true);
  });

  it("plaintext secret backend is flagged", () => {
    const c = cfg();
    c.secretBackend = "plaintext";
    const plan = getTarget("local").plan(c);
    expect(isSecure(plan)).toBe(false);
    expect(plan.warnings).toHaveLength(1);
    expect(plan.warnings[0]).toContain("plaintext");
  });

  it("warning ordering is stable", () => {
    const c = cfg();
    c.tls = false;
    c.denyAllEgress = false;
    c.secretBackend = "plaintext";
    const plan = getTarget("local").plan(c);
    expect(plan.warnings).toEqual([
      "TLS disabled -- traffic would be unencrypted",
      "egress not restricted -- data exfil risk",
      "secrets stored in plaintext",
    ]);
  });

  it("target selects the right secret backend", () => {
    expect(getTarget("azure").plan(cfg()).manifests["main.bicep"]).toContain("keyvault");
    expect(getTarget("aws").plan(cfg()).steps.join("\n")).toContain("secrets-manager");
    expect(getTarget("local").plan(cfg()).manifests["values.yaml"]).toContain("k8s-secret");
  });

  it("non-auto secret backend override propagates", () => {
    const c = cfg();
    c.secretBackend = "vault";
    const plan = getTarget("local").plan(c);
    expect(plan.manifests["values.yaml"]).toContain("secretBackend: vault");
  });

  it("unknown target throws with exact message", () => {
    expect(() => getTarget("gcp")).toThrowError(
      "unknown target 'gcp'. available: ['local', 'azure', 'aws']",
    );
  });

  it("renders helm values exactly", () => {
    const plan = getTarget("local").plan(cfg());
    expect(plan.manifests["values.yaml"]).toBe(
      "image: ghcr.io/example/ledgerrag:latest\nreplicas: 2\ntls: true\nsecretBackend: k8s-secret\nnetworkPolicy:\n  denyAllEgress: true\n",
    );
  });

  it("renders bicep exactly", () => {
    const plan = getTarget("azure").plan(cfg());
    expect(plan.manifests["main.bicep"]).toBe(
      "// Bicep for ledgerrag\nparam image string = 'ghcr.io/example/ledgerrag:latest'\nparam secretBackend string = 'keyvault'\n",
    );
  });

  it("renders terraform exactly", () => {
    const plan = getTarget("aws").plan(cfg());
    expect(plan.manifests["main.tf"]).toBe(
      '# Terraform for ledgerrag\nvariable "image" { default = "ghcr.io/example/ledgerrag:latest" }\n# secrets: secrets-manager\n',
    );
  });

  it("tls off changes ingress and front steps", () => {
    const c = cfg();
    c.tls = false;
    const azureSteps = getTarget("azure").plan(c).steps.join("\n");
    expect(azureSteps).not.toContain("with TLS");
    expect(azureSteps).toContain("enable ingress");
    const awsSteps = getTarget("aws").plan(c).steps.join("\n");
    expect(awsSteps).not.toContain("ACM TLS cert");
    expect(awsSteps).toContain("front with ALB");
  });

  it("deny egress off removes the network policy step", () => {
    const c = cfg();
    c.denyAllEgress = false;
    const steps = getTarget("local").plan(c).steps;
    expect(steps).toHaveLength(4);
    expect(steps.some((s) => s.includes("NetworkPolicy"))).toBe(false);
  });

  it("renders local steps exactly with defaults", () => {
    const steps = getTarget("local").plan(cfg()).steps;
    expect(steps).toEqual([
      "ensure kind/minikube cluster is running",
      "create namespace ledgerrag",
      "create k8s-secret for app credentials",
      "helm install ledgerrag ./charts/ledgerrag --set image=ghcr.io/example/ledgerrag:latest",
      "apply default-deny NetworkPolicy",
    ]);
  });

  it("render plan secure returns zero", () => {
    const result = renderPlan("local", cfg());
    expect(result.code).toBe(0);
    expect(result.text).toContain("secure-by-default checks: PASS");
    expect(result.text.startsWith("== DeployKit plan: local / ledgerrag ==\n")).toBe(true);
  });

  it("render plan insecure returns one", () => {
    const c = cfg();
    c.tls = false;
    c.denyAllEgress = false;
    const result = renderPlan("aws", c);
    expect(result.code).toBe(1);
    expect(result.text).toContain("SECURITY WARNINGS:");
  });

  it("render plan unknown target throws", () => {
    expect(() => renderPlan("gcp", cfg())).toThrowError();
  });

  it("target names are in order", () => {
    expect([...TARGET_NAMES]).toEqual(["local", "azure", "aws"]);
  });

  it("target name property is exposed", () => {
    expect(getTarget("local").name).toBe("local");
    expect(getTarget("azure").name).toBe("azure");
    expect(getTarget("aws").name).toBe("aws");
  });
});
